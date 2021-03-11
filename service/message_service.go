package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-state-types/abi"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-messager/config"
	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

const (
	MaxHeadChangeProcess = 5

	LookBackLimit = 1000

	maxStoreTipsetCount = 1000
)

type MessageService struct {
	repo           repo.Repo
	messageRepo    repo.MessageRepo
	addressRepo    repo.AddressRepo
	log            *logrus.Logger
	cfg            *config.MessageServiceConfig
	nodeClient     *NodeClient
	messageState   *MessageState
	addressService *AddressService

	triggerPush chan *venusTypes.TipSet
	headChans   chan *headChan

	tsCache *TipsetCache

	l sync.Mutex
}

type headChan struct {
	apply, revert []*venusTypes.TipSet
}

type TipsetCache struct {
	Cache      map[uint64]*tipsetFormat
	CurrHeight uint64
}

func NewMessageService(repo repo.Repo,
	nc *NodeClient,
	logger *logrus.Logger,
	cfg *config.MessageServiceConfig,
	messageState *MessageState,
	addressService *AddressService) (*MessageService, error) {
	ms := &MessageService{
		messageRepo:    repo.MessageRepo(),
		log:            logger,
		nodeClient:     nc,
		cfg:            cfg,
		headChans:      make(chan *headChan, MaxHeadChangeProcess),
		messageState:   messageState,
		addressService: addressService,
		tsCache: &TipsetCache{
			Cache:      make(map[uint64]*tipsetFormat, maxStoreTipsetCount),
			CurrHeight: 0,
		},
	}
	ms.refreshMessageState(context.TODO())

	return ms, nil
}

func (ms *MessageService) PushMessage(ctx context.Context, msg *types.Message) (types.UUID, error) {
	msg.State = types.UnFillMsg
	return ms.messageRepo.SaveMessage(msg)
}

func (ms *MessageService) GetMessage(ctx context.Context, uuid types.UUID) (*types.Message, error) {
	return ms.messageRepo.GetMessage(uuid)
}

func (ms *MessageService) GetMessageState(ctx context.Context, uuid types.UUID) (types.MessageState, error) {
	if msg, ok := ms.messageState.GetMessage(uuid.String()); ok {
		return msg.State, nil
	}
	return ms.messageRepo.GetMessageState(uuid)
}

func (ms *MessageService) GetMessageByCid(background context.Context, cid string) (*types.Message, error) {
	return ms.messageRepo.GetMessageByCid(cid)
}

func (ms *MessageService) ListMessage(ctx context.Context) ([]*types.Message, error) {
	return ms.messageRepo.ListMessage()
}

func (ms *MessageService) ProcessNewHead(ctx context.Context, apply, revert []*venusTypes.TipSet) error {
	ms.log.Infof("receive new head from chain")
	if !ms.cfg.IsProcessHead {
		ms.log.Infof("skip process new head")
		return nil
	}
	ms.headChans <- &headChan{
		apply:  apply,
		revert: revert,
	}
	return nil
}

func (ms *MessageService) ReconnectCheck(ctx context.Context, head *venusTypes.TipSet) error {
	ms.log.Infof("reconnect to node")

	tsCache, err := readTipsetFile(ms.cfg.TipsetFilePath)
	if err != nil {
		return xerrors.Errorf("read tipset info failed %v", err)
	}

	if len(tsCache.Cache) == 0 {
		return nil
	}

	tsList := ms.ListTs()
	sort.Sort(tsList)

	if tsList[0].Height == uint64(head.Height()) && tsList[0].Key == head.String() {
		ms.log.Infof("The head does not change and returns directly.")
		return nil
	}

	gapTipset, err := ms.lookAncestors(ctx, tsList, head)
	if err != nil {
		return err
	}

	if len(gapTipset) == 0 {
		return nil
	}

	// handle revert
	if tsList[0].Height > uint64(head.Height()) || (tsList[0].Height == uint64(head.Height()) && tsList[0].Key != head.String()) {
		if err := ms.findAndRecordRevertMsgs(gapTipset[0].Height()); err != nil {
			return err
		}
	}

	err = ms.doRefreshMessageState(ctx, &headChan{
		apply:  gapTipset,
		revert: nil,
	})

	return err
}

func (ms *MessageService) lookAncestors(ctx context.Context, localTipset tipsetList, head *venusTypes.TipSet) ([]*venusTypes.TipSet, error) {
	var err error

	ts := &venusTypes.TipSet{}
	*ts = *head

	localTs := localTipset[0]
	idx := 0
	localTsLen := len(localTipset)

	gapTipset := make([]*venusTypes.TipSet, 0)
	loopCount := 0
	for {
		if loopCount > LookBackLimit {
			break
		}
		if idx >= localTsLen {
			break
		}
		if ts.Height() == 0 {
			break
		}
		if localTs.Height > uint64(ts.Height()) {
			idx++
		} else if localTs.Height == uint64(ts.Height()) {
			if localTs.Key == ts.String() {
				break
			}
			idx++
		} else {
			gapTipset = append(gapTipset, ts)
			ts, err = ms.nodeClient.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("get tipset failed %v", err)
			}
		}
		loopCount++
	}

	return gapTipset, nil
}

func (ms *MessageService) RemoveTs(list []*tipsetFormat) {
	ms.l.Lock()
	defer ms.l.Unlock()
	for _, ts := range list {
		delete(ms.tsCache.Cache, ts.Height)
	}
}

func (ms *MessageService) AddTs(list ...*tipsetFormat) {
	ms.l.Lock()
	defer ms.l.Unlock()
	for _, ts := range list {
		ms.tsCache.Cache[ts.Height] = ts
	}
}

func (ms *MessageService) ExistTs(height uint64) bool {
	ms.l.Lock()
	defer ms.l.Unlock()
	_, ok := ms.tsCache.Cache[height]

	return ok
}

func (ms *MessageService) ReduceTs() {
	ms.l.Lock()
	defer ms.l.Unlock()
	minHeight := ms.tsCache.CurrHeight - maxStoreTipsetCount
	for _, v := range ms.tsCache.Cache {
		if v.Height < minHeight {
			delete(ms.tsCache.Cache, v.Height)
		}
	}
}

func (ms *MessageService) ListTs() tipsetList {
	ms.l.Lock()
	defer ms.l.Unlock()
	var list tipsetList
	for _, ts := range ms.tsCache.Cache {
		list = append(list, ts)
	}

	return list
}

func (ms *MessageService) findAndRecordRevertMsgs(height abi.ChainEpoch) error {
	msgs, err := ms.repo.MessageRepo().GetSignedMessageByHeight(height)
	if err != nil {
		return err
	}

	var cid string
	for _, msg := range msgs {
		cid = msg.Cid().String()
		ms.messageState.SetMessageState(cid, types.UnFillMsg)
		ms.messageState.idCids.Set(msg.ID.String(), cid)
		if err := ms.repo.MessageRepo().UpdateMessageStateByCid(cid, types.UnFillMsg); err != nil {
			return xerrors.Errorf("update message state failed, cid: %s, error: %v", cid, err)
		}
	}

	return nil
}

///   Message push    ////

func (ms *MessageService) pushMessageToPool(ctx context.Context, ts *venusTypes.TipSet) error {
	addrList, err := ms.addressService.ListAddress(ctx)
	if err != nil {
		return err
	}

	var toPushMessage []*venusTypes.SignedMessage
	for _, addr := range addrList {

		if err = ms.repo.Transaction(func(txRepo repo.TxRepo) error {
			addrInfo, _ := ms.addressService.GetAddressInfo(addr.Addr)

			mAddr, err := address.NewFromString(addr.Addr)
			if err != nil {
				return err
			}
			//判断是否需要推送消息
			actor, err := ms.nodeClient.StateGetActor(ctx, mAddr, ts.Key())
			if err != nil {
				return err
			}

			if actor.Nonce > addr.Nonce {
				//todo maybe a message create outof system, this should corrent in status check
				//todo or corrent here?
				ms.log.Warnf("%s nonce in db %d is smaller than nonce on chain %d", addr.Addr, addr.Nonce, actor.Nonce)
				return nil
			}
			nonceGap := addr.Nonce - actor.Nonce
			if nonceGap < 20 {
				ms.log.Debugf("%s there are %d message not to be package ", addr.Addr, nonceGap)
				return nil
			}
			selectCount := 20 - nonceGap
			//消息排序
			messages, err := txRepo.MessageRepo().ListUnChainMessageByAddress(mAddr)
			if err != nil {
				return err
			}
			if len(messages) == 0 {
				ms.log.Debugf("%s have no message", addr.Addr)
				return nil
			}
			messages, expireMsgs := ms.excludeExpire(ts, messages)
			//todo 如何筛选
			selectMsg := messages[:]
			if uint64(len(messages)) > selectCount {
				selectMsg = messages[:selectCount]
			}

			for _, msg := range selectMsg {
				//分配nonce
				addr.Nonce++
				msg.Nonce = addr.Nonce

				//todo 估算gas, spec怎么做？
				//通过配置影响 maxfee
				newMsg, err := ms.nodeClient.GasEstimateMessageGas(ctx, msg.VMMessage(), &venusTypes.MessageSendSpec{MaxFee: msg.Meta.MaxFee}, ts.Key())
				if err != nil {
					return err
				}
				msg.GasFeeCap = newMsg.GasFeeCap
				msg.GasPremium = newMsg.GasPremium
				msg.GasLimit = newMsg.GasLimit

				//签名
				mb, err := msg.ToStorageBlock()
				if err != nil {
					return xerrors.Errorf("serializing message: %w", err)
				}
				sig, err := addrInfo.WalletClient.WalletSign(ctx, mAddr, mb.RawData())
				if err != nil {
					return err
				}

				msg.Signature = sig
				msg.State = types.FillMsg

				unsignedCid := msg.UnsignedMessage.Cid()
				msg.UnsignedCid = &unsignedCid

				signedMsg := venusTypes.SignedMessage{
					Message:   msg.UnsignedMessage,
					Signature: *msg.Signature,
				}

				signedCid := signedMsg.Cid()
				msg.SignedCid = &signedCid
			}

			//保存消息
			//todo transaction
			err = txRepo.MessageRepo().ExpireMessage(expireMsgs)
			if err != nil {
				return err
			}

			err = txRepo.MessageRepo().BatchSaveMessage(selectMsg)
			if err != nil {
				return err
			}

			_, err = txRepo.AddressRepo().SaveAddress(ctx, addr)
			if err != nil {
				return err
			}
			for _, msg := range selectMsg {
				toPushMessage = append(toPushMessage, &venusTypes.SignedMessage{
					Message:   msg.UnsignedMessage,
					Signature: *msg.Signature,
				})
				//update cache
			}

			return nil
		}); err != nil {
			ms.log.Errorf("select message of %s failed %w", addr.Addr, err)
			return err
		}
	}

	//广播推送
	//todo 多点推送
	_, err = ms.nodeClient.MpoolBatchPush(ctx, toPushMessage)
	return err
}

func (ms *MessageService) excludeExpire(ts *venusTypes.TipSet, msgs []*types.Message) ([]*types.Message, []*types.Message) {
	//todo 判断过期
	var result []*types.Message
	var expireMsg []*types.Message
	for _, msg := range msgs {
		if msg.Meta.ExpireEpoch != 0 && msg.Meta.ExpireEpoch > ts.Height() {
			//expire
			msg.State = types.ExpireMsg
			expireMsg = append(expireMsg, msg)
			continue
		}
		result = append(result, msg)
	}
	return result, expireMsg
}

func (ms *MessageService) StartPushMessage(ctx context.Context) {
	tm := time.NewTicker(time.Second * 30)
	defer tm.Stop()

	for {
		select {
		case <-ctx.Done():
			ms.log.Infof("Stop push message")
		case <-tm.C:
			newHead, err := ms.nodeClient.ChainHead(ctx)
			if err != nil {
				ms.log.Errorf("fail to get chain head %w", err)
			}
			err = ms.pushMessageToPool(ctx, newHead)
			if err != nil {
				ms.log.Errorf("push message error %w", err)
			}
		case newHead := <-ms.triggerPush:
			err := ms.pushMessageToPool(ctx, newHead)
			if err != nil {
				ms.log.Errorf("push message error %w", err)
			}
		}
	}
}
