package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus-wallet/core"
	"github.com/filecoin-project/venus/pkg/messagepool"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	gatewayTypes "github.com/ipfs-force-community/venus-gateway/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/venus-messager/config"
	"github.com/filecoin-project/venus-messager/gateway"
	"github.com/filecoin-project/venus-messager/log"
	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
	"github.com/filecoin-project/venus-messager/utils"
)

var errAlreadyInMpool = xerrors.Errorf("already in mpool: %v", messagepool.ErrSoftValidationFailure)
var errMinimumNonce = "minimum expected nonce"

const (
	MaxHeadChangeProcess = 5

	LookBackLimit = 900

	maxStoreTipsetCount = 3000
)

type MessageService struct {
	repo           repo.Repo
	log            *log.Logger
	cfg            *config.MessageServiceConfig
	nodeClient     *NodeClient
	messageState   *MessageState
	addressService *AddressService
	walletClient   gateway.IWalletClient

	triggerPush chan *venusTypes.TipSet
	headChans   chan *headChan

	readFileOnce sync.Once
	tsCache      *TipsetCache

	messageSelector *MessageSelector

	sps         *SharedParamsService
	nodeService *NodeService

	preCancel context.CancelFunc
}

type headChan struct {
	apply, revert []*venusTypes.TipSet
	isReconnect   bool
	done          chan error
}

type TipsetCache struct {
	Cache      map[int64]*tipsetFormat
	CurrHeight int64

	l sync.Mutex
}

func NewMessageService(repo repo.Repo,
	nc *NodeClient,
	logger *log.Logger,
	cfg *config.MessageServiceConfig,
	messageState *MessageState,
	addressService *AddressService,
	sps *SharedParamsService,
	nodeService *NodeService,
	walletClient *gateway.IWalletCli) (*MessageService, error) {
	selector := NewMessageSelector(repo, logger, cfg, nc, addressService, sps, walletClient)
	ms := &MessageService{
		repo:            repo,
		log:             logger,
		nodeClient:      nc,
		cfg:             cfg,
		messageSelector: selector,
		headChans:       make(chan *headChan, MaxHeadChangeProcess),

		messageState:   messageState,
		addressService: addressService,
		walletClient:   walletClient,
		tsCache: &TipsetCache{
			Cache:      make(map[int64]*tipsetFormat, maxStoreTipsetCount),
			CurrHeight: 0,
		},
		triggerPush: make(chan *venusTypes.TipSet, 20),
		sps:         sps,
		nodeService: nodeService,
	}
	ms.refreshMessageState(context.TODO())

	return ms, nil
}

func (ms *MessageService) pushMessage(ctx context.Context, msg *types.Message) error {
	if len(msg.ID) == 0 {
		return xerrors.New("empty uid")
	}

	//replace address
	if msg.From.Protocol() == address.ID {
		fromA, err := ms.nodeClient.StateAccountKey(ctx, msg.From, venusTypes.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting key address: %w", err)
		}
		ms.log.Warnf("Push from ID address (%s), adjusting to %s", msg.From, fromA)
		msg.From = fromA
	}

	has, err := ms.walletClient.WalletHas(ctx, msg.WalletName, msg.From)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("wallet(%s) address %s not exists", msg.WalletName, msg.From)
	}
	var addrInfo *types.Address
	if err := ms.repo.Transaction(func(txRepo repo.TxRepo) error {
		addrInfo, err = ms.addressService.GetAddress(ctx, msg.From)
		if err == nil {
			return nil
		}
		if xerrors.Is(err, gorm.ErrRecordNotFound) {
			if err = ms.repo.AddressRepo().SaveAddress(ctx, &types.Address{
				ID:        types.NewUUID(),
				Addr:      msg.From,
				Nonce:     0,
				State:     types.Alive,
				IsDeleted: repo.NotDeleted,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}); err != nil {
				return xerrors.Errorf("save address %s failed %v", msg.From.String(), err)
			}
			ms.log.Infof("add new address %s", msg.From.String())
		}
		return err
	}); err != nil {
		return err
	}
	if addrInfo.State == types.Forbiden {
		ms.log.Errorf("address(%s) is forbidden", msg.From.String())
		return xerrors.Errorf("address(%s) is forbidden", msg.From.String())
	}

	msg.Nonce = 0
	err = ms.repo.MessageRepo().CreateMessage(msg)
	if err == nil {
		ms.messageState.SetMessage(msg.ID, msg)
	}

	return err
}

func ipAccountFromContext(ctx context.Context) (string, string) {
	ip, _ := ctx.Value(gatewayTypes.IPKey).(string)
	account, _ := ctx.Value(gatewayTypes.AccountKey).(string)

	return ip, account
}

func (ms *MessageService) PushMessage(ctx context.Context, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta) (string, error) {
	newId := types.NewUUID()
	_, account := ipAccountFromContext(ctx)

	return newId.String(), ms.pushMessage(ctx, &types.Message{
		ID:              newId.String(),
		UnsignedMessage: *msg,
		Meta:            meta,
		State:           types.UnFillMsg,
		WalletName:      account,
		FromUser:        account,
	})
}

func (ms *MessageService) PushMessageWithId(ctx context.Context, id string, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta) (string, error) {
	_, account := ipAccountFromContext(ctx)
	return id, ms.pushMessage(ctx, &types.Message{
		ID:              id,
		UnsignedMessage: *msg,
		Meta:            meta,
		State:           types.UnFillMsg,
		WalletName:      account,
		FromUser:        account,
	})
}

func (ms *MessageService) WaitMessage(ctx context.Context, id string, confidence uint64) (*types.Message, error) {
	tm := time.NewTicker(time.Second * 30)
	defer tm.Stop()

	doneCh := make(chan struct{}, 1)
	doneCh <- struct{}{}

	for {
		select {
		case <-doneCh:
			msg, err := ms.GetMessageByUid(ctx, id)
			if err != nil {
				return nil, err
			}

			switch msg.State {
			//OffChain
			case types.FillMsg:
				fallthrough
			case types.UnFillMsg:
				fallthrough
			case types.UnKnown:
				continue
			//OnChain
			case types.ReplacedMsg:
				fallthrough
			case types.OnChainMsg:
				if msg.Confidence > int64(confidence) {
					return msg, nil
				}
				continue
			//Error
			case types.FailedMsg:
				return msg, nil
			case types.NoWalletMsg:
				return nil, xerrors.New("msg failed due to wallet disappear")
			}

		case <-tm.C:
			doneCh <- struct{}{}
		case <-ctx.Done():
			return nil, xerrors.New("exit by client ")
		}
	}
}

func (ms *MessageService) GetMessageByUid(ctx context.Context, id string) (*types.Message, error) {
	ts, err := ms.nodeClient.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := ms.repo.MessageRepo().GetMessageByUid(id)
	if err != nil {
		return nil, err
	}
	if msg.State == types.OnChainMsg {
		msg.Confidence = int64(ts.Height()) - msg.Height
	}
	return msg, nil
}

func (ms *MessageService) HasMessageByUid(ctx context.Context, id string) (bool, error) {
	return ms.repo.MessageRepo().HasMessageByUid(id)
}

func (ms *MessageService) GetMessageByCid(ctx context.Context, id cid.Cid) (*types.Message, error) {
	ts, err := ms.nodeClient.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := ms.repo.MessageRepo().GetMessageByCid(id)
	if err != nil {
		return nil, err
	}
	if msg.State == types.OnChainMsg {
		msg.Confidence = int64(ts.Height()) - msg.Height
	}
	return msg, nil
}

func (ms *MessageService) GetMessageState(ctx context.Context, id string) (types.MessageState, error) {
	return ms.repo.MessageRepo().GetMessageState(id)
}

func (ms *MessageService) GetMessageBySignedCid(ctx context.Context, signedCid cid.Cid) (*types.Message, error) {
	return ms.repo.MessageRepo().GetMessageBySignedCid(signedCid)
}

func (ms *MessageService) GetMessageByUnsignedCid(ctx context.Context, unsignedCid cid.Cid) (*types.Message, error) {
	return ms.repo.MessageRepo().GetMessageByCid(unsignedCid)
}

func (ms *MessageService) GetMessageByFromAndNonce(ctx context.Context, from address.Address, nonce uint64) (*types.Message, error) {
	return ms.repo.MessageRepo().GetMessageByFromAndNonce(from, nonce)
}

func (ms *MessageService) ListMessageByFromState(ctx context.Context, from address.Address, state types.MessageState, pageIndex, pageSize int) ([]*types.Message, error) {
	return ms.repo.MessageRepo().ListMessageByFromState(from, state, pageIndex, pageSize)
}

func (ms *MessageService) ListMessage(ctx context.Context) ([]*types.Message, error) {
	ts, err := ms.nodeClient.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	msgs, err := ms.repo.MessageRepo().ListMessage()
	if err != nil {
		return nil, err
	}

	for _, msg := range msgs {
		if msg.State == types.OnChainMsg {
			msg.Confidence = int64(ts.Height()) - msg.Height
		}
	}
	return msgs, nil
}

func (ms *MessageService) ListMessageByAddress(ctx context.Context, addr address.Address) ([]*types.Message, error) {
	ts, err := ms.nodeClient.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	msgs, err := ms.repo.MessageRepo().ListMessageByAddress(addr)
	if err != nil {
		return nil, err
	}

	for _, msg := range msgs {
		if msg.State == types.OnChainMsg {
			msg.Confidence = int64(ts.Height()) - msg.Height
		}
	}
	return msgs, nil
}

func (ms *MessageService) ListFailedMessage(ctx context.Context) ([]*types.Message, error) {
	return ms.repo.MessageRepo().ListFailedMessage()
}

func (ms *MessageService) ListFilledMessageByAddress(ctx context.Context, addr address.Address) ([]*types.Message, error) {
	msgs, err := ms.repo.MessageRepo().ListFilledMessageByAddress(addr)
	if len(msgs) > 0 {
		ids := make([]string, 0, len(msgs))
		for _, msg := range msgs {
			ids = append(ids, msg.ID)
		}
		ms.log.Errorf("list failed message %d %s", len(ids), strings.Join(ids, ","))
	}

	return msgs, err
}

func (ms *MessageService) ListBlockedMessage(ctx context.Context, addr address.Address, d time.Duration) ([]*types.Message, error) {
	var msgs []*types.Message
	var err error
	if addr != address.Undef {
		msgs, err = ms.repo.MessageRepo().ListBlockedMessage(addr, d)
	} else {
		addrList, err := ms.addressService.ListAddress(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range addrList {
			msgsT, err := ms.repo.MessageRepo().ListBlockedMessage(a.Addr, d)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msgsT...)
		}
	}

	if len(msgs) > 0 {
		ids := make([]string, 0, len(msgs))
		for _, msg := range msgs {
			ids = append(ids, msg.ID)
		}
		ms.log.Errorf("list blocked message %d %s", len(ids), strings.Join(ids, ","))
	}

	return msgs, err
}

func (ms *MessageService) UpdateMessageStateByCid(ctx context.Context, cid string, state types.MessageState) (string, error) {
	return cid, ms.repo.MessageRepo().UpdateMessageStateByCid(cid, state)
}

func (ms *MessageService) UpdateMessageStateByID(ctx context.Context, id string, state types.MessageState) (string, error) {
	return id, ms.repo.MessageRepo().UpdateMessageStateByID(id, state)
}

func (ms *MessageService) UpdateMessageInfoByCid(unsignedCid string, receipt *venusTypes.MessageReceipt,
	height abi.ChainEpoch, state types.MessageState, tsKey venusTypes.TipSetKey) (string, error) {
	return unsignedCid, ms.repo.MessageRepo().UpdateMessageInfoByCid(unsignedCid, receipt, height, state, tsKey)
}

func (ms *MessageService) ProcessNewHead(ctx context.Context, apply, revert []*venusTypes.TipSet) error {
	ms.log.Infof("receive new head from chain")
	if ms.cfg.SkipProcessHead {
		ms.log.Infof("skip process new head")
		return nil
	}

	if len(apply) == 0 {
		ms.log.Errorf("expect apply blocks, but got none")
		return nil
	}

	ts := ms.tsCache.ListTs()
	sort.Sort(ts)
	smallestTs := apply[len(apply)-1]

	defer ms.log.Infof("%d head wait to process", len(ms.headChans))

	if ts == nil || smallestTs.Parents().String() == ts[0].Key {
		ms.log.Infof("apply a block height %d %s", apply[0].Height(), apply[0].String())
		done := make(chan error)
		ms.headChans <- &headChan{
			apply:  apply,
			revert: nil,
			done:   done,
		}
		return <-done
	} else {
		apply, revertTipset, err := ms.lookAncestors(ctx, ts, smallestTs)
		if err != nil {
			ms.log.Errorf("look ancestor error from %s and %s", smallestTs, ts[0].Key)
			return nil
		}

		done := make(chan error)
		ms.headChans <- &headChan{
			apply:  apply,
			revert: revertTipset,
			done:   done,
		}
		return <-done
	}
}

func (ms *MessageService) ReconnectCheck(ctx context.Context, head *venusTypes.TipSet) error {
	ms.log.Infof("reconnect to node")

	ms.readFileOnce.Do(func() {
		tsCache, err := readTipsetFile(ms.cfg.TipsetFilePath)
		if err != nil {
			ms.log.Errorf("read tipset file failed %v", err)
		} else {
			ms.tsCache = tsCache
		}
	})

	if len(ms.tsCache.Cache) == 0 {
		return nil
	}

	tsList := ms.tsCache.ListTs()
	sort.Sort(tsList)

	// long time not use
	if int64(head.Height())-tsList[0].Height >= LookBackLimit {
		count, err := ms.UpdateAllFilledMessage(ctx)
		if err != nil {
			return err
		}
		ms.log.Infof("gap height %v, update filled message count %v", int64(head.Height())-tsList[0].Height, count)
		return nil
	}

	if tsList[0].Height == int64(head.Height()) && tsList[0].Key == head.String() {
		ms.log.Infof("The head does not change and returns directly.")
		return nil
	}

	gapTipset, revertTipset, err := ms.lookAncestors(ctx, tsList, head)
	if err != nil {
		return err
	}

	done := make(chan error)
	ms.headChans <- &headChan{
		apply:       gapTipset,
		revert:      revertTipset,
		done:        done,
		isReconnect: true,
	}

	return <-done
}

func (ms *MessageService) lookAncestors(ctx context.Context, localTipset tipsetList, head *venusTypes.TipSet) ([]*venusTypes.TipSet, []*venusTypes.TipSet, error) {
	var err error

	ts := &venusTypes.TipSet{}
	*ts = *head

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
		localTs := localTipset[idx]

		if ts.Height() == 0 {
			break
		}
		if localTs.Height > int64(ts.Height()) {
			idx++
		} else if localTs.Height == int64(ts.Height()) {
			if localTs.Key == ts.String() {
				break
			}
			idx++
		} else {
			gapTipset = append(gapTipset, ts)
			ts, err = ms.nodeClient.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				return nil, nil, xerrors.Errorf("get tipset failed %v", err)
			}
		}
		loopCount++
	}

	var revertTsf []*tipsetFormat
	if idx >= localTsLen {
		idx = localTsLen
	}
	revertTsf = localTipset[:idx]

	revertTs, err := ms.convertTipsetFormatToTipset(revertTsf)

	return gapTipset, revertTs, err
}

func (ms *MessageService) convertTipsetFormatToTipset(tf []*tipsetFormat) ([]*venusTypes.TipSet, error) {
	var tsList []*venusTypes.TipSet
	var err error
	for _, t := range tf {
		key, err := utils.StringToTipsetKey(t.Key)
		if err != nil {
			return nil, err
		}
		blocks := make([]*venusTypes.BlockHeader, len(key.Cids()))
		for i, cid := range key.Cids() {
			blocks[i], err = ms.nodeClient.ChainGetBlock(context.TODO(), cid)
			if err != nil {
				return nil, err
			}
		}
		ts, err := venusTypes.NewTipSet(blocks...)
		if err != nil {
			return nil, err
		}
		tsList = append(tsList, ts)
	}

	return tsList, err
}

///   Message push    ////

func (ms *MessageService) pushMessageToPool(ctx context.Context, ts *venusTypes.TipSet) error {
	// select message
	tSelect := time.Now()
	selectResult, err := ms.messageSelector.SelectMessage(ctx, ts)
	if err != nil {
		return err
	}
	ms.log.Infof("current loop select result | SelectMsg: %d | ExpireMsg: %d | ToPushMsg: %d | ErrMsg: %d", len(selectResult.SelectMsg), len(selectResult.ExpireMsg), len(selectResult.ToPushMsg), len(selectResult.ErrMsg))
	tSaveDb := time.Now()
	ms.log.Infof("start to save to database")
	//save to db
	if err = ms.repo.Transaction(func(txRepo repo.TxRepo) error {
		//保存消息
		err = txRepo.MessageRepo().ExpireMessage(selectResult.ExpireMsg)
		if err != nil {
			return err
		}

		err = txRepo.MessageRepo().BatchSaveMessage(selectResult.SelectMsg)
		if err != nil {
			return err
		}

		for _, addr := range selectResult.ModifyAddress {
			err = txRepo.AddressRepo().UpdateNonce(ctx, addr.Addr, addr.Nonce)
			if err != nil {
				return err
			}
		}

		for _, m := range selectResult.ErrMsg {
			ms.log.Infof("update err message %s", m.id)
			err := txRepo.MessageRepo().UpdateReturnValue(m.id, m.err)
			if err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		ms.log.Errorf("save signed message failed %v", err)
		return err
	}

	ms.log.Infof("success to save to database")

	tCacheUpdate := time.Now()
	ms.log.Infof("success to update memory cache")
	//update cache
	for _, msg := range selectResult.SelectMsg {
		selectResult.ToPushMsg = append(selectResult.ToPushMsg, &venusTypes.SignedMessage{
			Message:   msg.UnsignedMessage,
			Signature: *msg.Signature,
		})
		//update cache
		err := ms.messageState.MutatorMessage(msg.ID, func(message *types.Message) error {
			message.SignedCid = msg.SignedCid
			message.UnsignedCid = msg.UnsignedCid
			message.UnsignedMessage = msg.UnsignedMessage
			message.State = msg.State
			message.Signature = msg.Signature
			message.Nonce = msg.Nonce
			if message.Receipt != nil {
				message.Receipt.ReturnValue = nil //cover data for err before
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	for _, m := range selectResult.ErrMsg {
		err := ms.messageState.MutatorMessage(m.id, func(message *types.Message) error {
			if message.Receipt != nil {
				message.Receipt.ReturnValue = []byte(m.err)
			} else {
				message.Receipt = &venusTypes.MessageReceipt{ReturnValue: []byte(m.err)}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	//broad cast  push to node in config ,push to multi node in db config
	go func() {
		tPush := time.Now()
		ms.log.Infof("start to push message %d to mpool", len(selectResult.ToPushMsg))
		for _, msg := range selectResult.ToPushMsg {
			if _, pushErr := ms.nodeClient.MpoolPush(ctx, msg); err != nil {
				if !(strings.Contains(err.Error(), errMinimumNonce) || strings.Contains(err.Error(), errAlreadyInMpool.Error())) {
					ms.log.Errorf("push to message %s from: %s, nonce: %d, error  %v", msg.Message.Cid().String(), msg.Message.From, msg.Message.Nonce, pushErr)
				}
			}
		}

		ms.multiNodeToPush(ctx, selectResult.ToPushMsg)

		ms.log.Infof("Push message select time:%d , save db time:%d ,update cache time:%d, push time: %d",
			time.Since(tSelect).Milliseconds(),
			time.Since(tSaveDb).Milliseconds(),
			time.Since(tCacheUpdate).Milliseconds(),
			time.Since(tPush).Milliseconds(),
		)
	}()
	return err
}

type nodeClient struct {
	name  string
	cli   *NodeClient
	close jsonrpc.ClientCloser
}

func (ms *MessageService) multiNodeToPush(ctx context.Context, msgs []*venusTypes.SignedMessage) {
	if len(msgs) == 0 {
		ms.log.Warnf("no broadcast node config")
		return
	}

	nodeList, err := ms.nodeService.ListNode(context.TODO())
	if err != nil {
		ms.log.Errorf("list node %v", err)
		return
	}

	nc := make([]nodeClient, 0, len(nodeList))
	for _, node := range nodeList {
		cli, closer, err := NewNodeClient(context.TODO(), &config.NodeConfig{Token: node.Token, Url: node.URL})
		if err != nil {
			ms.log.Warnf("connect node(%s) %v", node.Name, err)
			continue
		}
		nc = append(nc, nodeClient{name: node.Name, cli: cli, close: closer})
	}

	if len(nc) == 0 {
		ms.log.Warnf("no available broadcast node config")
		return
	}

	fromMap := make(map[address.Address]struct{})
	for _, msg := range msgs {
		if _, ok := fromMap[msg.Message.From]; !ok {
			fromMap[msg.Message.From] = struct{}{}
		}
	}

	for _, node := range nc {
		for _, msg := range msgs {
			if _, err := node.cli.MpoolPush(ctx, msg); err != nil {
				//skip error
				if !(strings.Contains(err.Error(), errMinimumNonce) || strings.Contains(err.Error(), errAlreadyInMpool.Error())) {
					ms.log.Errorf("push message to node %s %v", node.name, err)
				}
			}
		}
		ms.log.Infof("start to broadcast message of address")
		for fromAddr := range fromMap {
			if err := node.cli.MpoolPublishByAddr(ctx, fromAddr); err != nil {
				ms.log.Errorf("publish message of address %s to node %s fail %v", fromAddr, node.name, err)
			}
		}

		node.close()
	}
}

func (ms *MessageService) StartPushMessage(ctx context.Context) {
	tm := time.NewTicker(time.Second * 30)
	defer tm.Stop()

	for {
		select {
		case <-ctx.Done():
			ms.log.Infof("Stop push message")
			return
		case <-tm.C:
			//newHead, err := ms.nodeClient.ChainHead(ctx)
			//if err != nil {
			//	ms.log.Errorf("fail to get chain head %v", err)
			//}
			//err = ms.pushMessageToPool(ctx, newHead)
			//if err != nil {
			//	ms.log.Errorf("push message error %v", err)
			//}
		case newHead := <-ms.triggerPush:
			// Receiving a channel `resetAddressFunc`, then reset the address
			ms.resetAddress()

			start := time.Now()
			ms.log.Infof("start to push message %s task wait task %d", newHead.String(), len(ms.triggerPush))
			err := ms.pushMessageToPool(ctx, newHead)
			if err != nil {
				ms.log.Errorf("push message error %v", err)
			}
			ms.log.Infof("end push message spent %d ms", time.Since(start).Milliseconds())
		}
	}
}

func (ms *MessageService) resetAddress() {
	select {
	case f := <-ms.addressService.resetAddressFunc:
		nonce, err := f()
		ms.addressService.resetAddressRes <- resetAddressResult{
			latestNonce: nonce,
			err:         err,
		}
	default:
	}
}

func (ms *MessageService) UpdateAllFilledMessage(ctx context.Context) (int, error) {
	msgs := make([]*types.Message, 0)

	for addr := range ms.addressService.Addresses() {
		filledMsgs, err := ms.repo.MessageRepo().ListFilledMessageByAddress(addr)
		if err != nil {
			ms.log.Errorf("list filled message %v %v", addr, err)
			continue
		}
		msgs = append(msgs, filledMsgs...)
	}

	ms.log.Infof("%d messages need to sync", len(msgs))
	updateCount := 0
	for _, msg := range msgs {
		if err := ms.updateFilledMessage(ctx, msg); err != nil {
			ms.log.Errorf("update filled message %v", err)
			continue
		}
		updateCount++
	}

	return updateCount, nil
}

func (ms *MessageService) updateFilledMessage(ctx context.Context, msg *types.Message) error {
	cid := msg.SignedCid
	if cid != nil {
		msgLookup, err := ms.nodeClient.StateSearchMsg(ctx, *cid)
		if err != nil || msgLookup == nil {
			return xerrors.Errorf("search message %s from node %v", cid.String(), err)
		}
		if _, err := ms.UpdateMessageInfoByCid(msg.UnsignedCid.String(), &msgLookup.Receipt, msgLookup.Height, types.OnChainMsg, msgLookup.TipSet); err != nil {
			return err
		}
		ms.log.Infof("update message %v by node success, height: %d", msg.ID, msgLookup.Height)
	}

	return nil
}

func (ms *MessageService) UpdateFilledMessageByID(ctx context.Context, id string) (string, error) {
	msg, err := ms.GetMessageByUid(ctx, id)
	if err != nil {
		return id, err
	}

	return id, ms.updateFilledMessage(ctx, msg)
}

func (ms *MessageService) ReplaceMessage(ctx context.Context, id string, auto bool, maxFee string, gasLimit int64, gasPremium string, gasFeecap string) (cid.Cid, error) {
	msg, err := ms.GetMessageByUid(ctx, id)
	if err != nil {
		return cid.Undef, xerrors.Errorf("found message %v", err)
	}
	if msg.State == types.OnChainMsg {
		return cid.Undef, xerrors.Errorf("message already on chain")
	}

	if auto {
		minRBF := messagepool.ComputeMinRBF(msg.GasPremium)

		var mss *venusTypes.MessageSendSpec
		if len(maxFee) > 0 {
			maxFee, err := venusTypes.BigFromString(maxFee)
			if err != nil {
				return cid.Undef, fmt.Errorf("parsing max-spend: %w", err)
			}
			mss = &venusTypes.MessageSendSpec{
				MaxFee: maxFee,
			}
		}

		// msg.GasLimit = 0 // TODO: need to fix the way we estimate gas limits to account for the messages already being in the mempool
		msg.GasFeeCap = abi.NewTokenAmount(0)
		msg.GasPremium = abi.NewTokenAmount(0)
		retm, err := ms.nodeClient.GasEstimateMessageGas(ctx, &msg.UnsignedMessage, mss, venusTypes.EmptyTSK)
		if err != nil {
			return cid.Undef, fmt.Errorf("failed to estimate gas values: %w", err)
		}

		msg.GasPremium = big.Max(retm.GasPremium, minRBF)
		msg.GasFeeCap = big.Max(retm.GasFeeCap, msg.GasPremium)

		mff := func() (abi.TokenAmount, error) {
			return abi.TokenAmount(venusTypes.DefaultDefaultMaxFee), nil
		}

		messagepool.CapGasFee(mff, &msg.UnsignedMessage, mss)
	} else {
		if gasLimit > 0 {
			msg.GasLimit = gasLimit
		}
		msg.GasPremium, err = venusTypes.BigFromString(gasPremium)
		if err != nil {
			return cid.Undef, fmt.Errorf("parsing gas-premium: %w", err)
		}
		// TODO: estimate fee cap here
		msg.GasFeeCap, err = venusTypes.BigFromString(gasFeecap)
		if err != nil {
			return cid.Undef, fmt.Errorf("parsing gas-feecap: %w", err)
		}
	}

	signedMsg, err := ToSignedMsg(ctx, ms.walletClient, msg)
	if err != nil {
		return cid.Undef, err
	}

	if err := ms.repo.MessageRepo().SaveMessage(msg); err != nil {
		return cid.Undef, err
	}
	err = ms.messageState.MutatorMessage(msg.ID, func(message *types.Message) error {
		message.SignedCid = msg.SignedCid
		message.GasLimit = msg.GasLimit
		message.GasPremium = msg.GasPremium
		message.GasFeeCap = msg.GasFeeCap
		message.UnsignedCid = msg.UnsignedCid
		message.UnsignedMessage = msg.UnsignedMessage
		message.State = msg.State
		message.Signature = msg.Signature
		message.Nonce = msg.Nonce
		return nil
	})
	if err != nil {
		return cid.Undef, err
	}

	_, err = ms.nodeClient.MpoolBatchPush(ctx, []*venusTypes.SignedMessage{&signedMsg})

	return signedMsg.Cid(), err
}

func (ms *MessageService) MarkBadMessage(ctx context.Context, id string) (struct{}, error) {
	return ms.repo.MessageRepo().MarkBadMessage(id)
}

func (ms *MessageService) RepublishMessage(ctx context.Context, id string) (struct{}, error) {
	msg, err := ms.GetMessageByUid(ctx, id)
	if err != nil {
		return struct{}{}, nil
	}
	if msg.State == types.OnChainMsg {
		return struct{}{}, xerrors.Errorf("message already on chain")
	}
	if msg.State != types.FillMsg {
		return struct{}{}, xerrors.Errorf("need FillMsg got %s", types.MsgStateToString(msg.State))
	}
	signedMsg := &venusTypes.SignedMessage{
		Message:   msg.UnsignedMessage,
		Signature: *msg.Signature,
	}
	if _, err := ms.nodeClient.MpoolPush(ctx, signedMsg); err != nil {
		return struct{}{}, err
	}
	ms.multiNodeToPush(ctx, []*venusTypes.SignedMessage{signedMsg})

	return struct{}{}, nil
}

func ToSignedMsg(ctx context.Context, walletCli gateway.IWalletClient, msg *types.Message) (venusTypes.SignedMessage, error) {
	unsignedCid := msg.UnsignedMessage.Cid()
	msg.UnsignedCid = &unsignedCid
	//签名
	data, err := msg.UnsignedMessage.ToStorageBlock()
	if err != nil {
		return venusTypes.SignedMessage{}, xerrors.Errorf("calc message unsigned message id %s fail %v", msg.ID, err)
	}
	sig, err := walletCli.WalletSign(ctx, msg.WalletName, msg.From, unsignedCid.Bytes(), core.MsgMeta{
		Type:  core.MTChainMsg,
		Extra: data.RawData(),
	})
	if err != nil {
		return venusTypes.SignedMessage{}, xerrors.Errorf("wallet sign failed %s fail %v", msg.ID, err)
	}

	msg.Signature = sig
	//state
	msg.State = types.FillMsg

	signedMsg := venusTypes.SignedMessage{
		Message:   msg.UnsignedMessage,
		Signature: *msg.Signature,
	}
	signedCid := signedMsg.Cid()
	msg.SignedCid = &signedCid

	return signedMsg, nil
}
