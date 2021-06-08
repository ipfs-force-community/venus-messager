package service

import (
	"context"
	"github.com/filecoin-project/go-jsonrpc"
	"modernc.org/mathutil"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus-wallet/core"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/venus/pkg/crypto"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/venus-messager/config"
	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
)

const (
	gasEstimate = "gas estimate: "
	signMsg     = "sign msg: "
)

type MessageSelector struct {
	repo           repo.Repo
	log            *logrus.Logger
	cfg            *config.MessageServiceConfig
	nodeClient     *NodeClient
	addressService *AddressService
	walletService  *WalletService
	sps            *SharedParamsService

	addrInfos *addrInfos
}

type addrInfos struct {
	addrInfos map[string]addrInfo

	l sync.Mutex
}

type addrInfo struct {
	walletCli IWalletClient
	close     jsonrpc.ClientCloser
	state     types.State
	err       error
}

type MsgSelectResult struct {
	SelectMsg     []*types.Message
	ExpireMsg     []*types.Message
	ToPushMsg     []*venusTypes.SignedMessage
	ModifyAddress []*types.Address
	ErrMsg        []msgErrInfo
}

type msgErrInfo struct {
	id  string
	err string
}

func NewMessageSelector(repo repo.Repo,
	log *logrus.Logger,
	cfg *config.MessageServiceConfig,
	nodeClient *NodeClient,
	addressService *AddressService,
	walletService *WalletService,
	sps *SharedParamsService) *MessageSelector {
	return &MessageSelector{repo: repo,
		log:            log,
		cfg:            cfg,
		nodeClient:     nodeClient,
		addressService: addressService,
		walletService:  walletService,
		sps:            sps,
	}
}

func (messageSelector *MessageSelector) SelectMessage(ctx context.Context, ts *venusTypes.TipSet) (*MsgSelectResult, error) {
	addrList, err := messageSelector.addressService.ListAddress(ctx)
	if err != nil {
		return nil, err
	}
	messageSelector.addrInfos = &addrInfos{
		addrInfos: make(map[string]addrInfo),
	}
	defer messageSelector.clearAddrInfos()

	appliedNonce, err := messageSelector.getNonceInTipset(ctx, ts)
	if err != nil {
		return nil, err
	}
	//sort by addr weight
	sort.Slice(addrList, func(i, j int) bool {
		return addrList[i].Weight < addrList[j].Weight
	})

	messageSelector.log.Infof("%d address wait to process", len(addrList))
	selectResult := &MsgSelectResult{}
	var lk sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	wg.Add(len(addrList))
	for _, addr := range addrList {
		go func(addr *types.Address) {
			sem <- struct{}{}
			defer func() {
				wg.Done()
				<-sem
			}()

			addrSelResult, err := messageSelector.selectAddrMessage(ctx, appliedNonce, addr, ts)
			if err != nil {
				messageSelector.log.Errorf("select message of %s fail %v", addr.Addr, err)
				return
			}
			lk.Lock()
			defer lk.Unlock()

			selectResult.ExpireMsg = append(selectResult.ExpireMsg, addrSelResult.ExpireMsg...)
			selectResult.ToPushMsg = append(selectResult.ToPushMsg, addrSelResult.ToPushMsg...)
			if len(addrSelResult.SelectMsg) > 0 {
				selectResult.SelectMsg = append(selectResult.SelectMsg, addrSelResult.SelectMsg...)
				selectResult.ModifyAddress = append(selectResult.ModifyAddress, addr)
			}
			selectResult.ErrMsg = append(selectResult.ErrMsg, addrSelResult.ErrMsg...)
		}(addr)
	}

	wg.Wait()

	return selectResult, nil
}

func (messageSelector *MessageSelector) selectAddrMessage(ctx context.Context, appliedNonce *types.NonceMap, addr *types.Address, ts *venusTypes.TipSet) (*MsgSelectResult, error) {
	var toPushMessage []*venusTypes.SignedMessage

	addrsInfo, err := messageSelector.repo.WalletAddressRepo().GetWalletAddressByAddrID(addr.ID)
	if err != nil {
		return nil, xerrors.Errorf("got wallet address by addr id %v", err)
	}
	if len(addrsInfo) == 0 {
		return nil, xerrors.Errorf("no wallet client")
	}

	var maxAllowPendingMessage uint64
	if messageSelector.sps.GetParams().SharedParams != nil {
		maxAllowPendingMessage = messageSelector.sps.GetParams().SelMsgNum
	}
	for _, addrInfo := range addrsInfo {
		if addrInfo.SelMsgNum != 0 {
			maxAllowPendingMessage = addrInfo.SelMsgNum
			break
		}
	}

	//判断是否需要推送消息
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	actorI, err := handleTimeout(messageSelector.nodeClient.StateGetActor, timeoutCtx, []interface{}{addr.Addr, ts.Key()})
	if err != nil {
		return nil, err
	}
	actor := actorI.(*venusTypes.Actor)
	nonceInLatestTs := actor.Nonce
	if nonceInTs, ok := appliedNonce.Get(addr.Addr); ok {
		messageSelector.log.Infof("update address %s nonce in ts %d  nonce in actor %d", addr.Addr, nonceInTs, nonceInLatestTs)
		nonceInLatestTs = nonceInTs
	}
	if nonceInLatestTs > addr.Nonce {
		messageSelector.log.Warnf("%s nonce in db %d is smaller than nonce on chain %d, update to latest", addr.Addr, addr.Nonce, nonceInLatestTs)
		addr.Nonce = nonceInLatestTs
		addr.UpdatedAt = time.Now()
		err := messageSelector.repo.AddressRepo().SaveAddress(ctx, addr)
		if err != nil {
			return nil, xerrors.Errorf("update address %s nonce fail", addr.Addr)
		}
	}
	//todo push signed but not onchain message, when to resend message
	filledMessage, err := messageSelector.repo.MessageRepo().ListFilledMessageByAddress(addr.Addr)
	if err != nil {
		messageSelector.log.Warnf("list filled message %v", err)
	}
	for _, msg := range filledMessage {
		if nonceInLatestTs > msg.Nonce {
			continue
		}
		toPushMessage = append(toPushMessage, &venusTypes.SignedMessage{
			Message:   msg.UnsignedMessage,
			Signature: *msg.Signature,
		})
	}

	//calc the message needed
	nonceGap := addr.Nonce - nonceInLatestTs
	if nonceGap >= maxAllowPendingMessage {
		messageSelector.log.Infof("%s there are %d message not to be package ", addr.Addr, nonceGap)
		return &MsgSelectResult{
			ToPushMsg: toPushMessage,
		}, nil
	}
	wantCount := maxAllowPendingMessage - nonceGap
	messageSelector.log.Infof("address %s pre state actor nonce %d, latest nonce %d, assigned nonce %d, nonce gap %d, want %d", addr.Addr, actor.Nonce, nonceInLatestTs, addr.Nonce, nonceGap, wantCount)
	//get message
	selectCount := mathutil.MinUint64(wantCount*2, 100)
	messages, err := messageSelector.repo.MessageRepo().ListUnChainMessageByAddress(addr.Addr, int(selectCount))
	if err != nil {
		return nil, xerrors.Errorf("list %s unpackage message error %v", addr.Addr, err)
	}

	//exclude expire message
	messages, expireMsgs := messageSelector.excludeExpire(ts, messages)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Meta.ExpireEpoch < messages[j].Meta.ExpireEpoch
	})

	//todo 如何筛选
	if len(messages) == 0 {
		messageSelector.log.Infof("%s have no message", addr.Addr)
		return &MsgSelectResult{
			ExpireMsg: expireMsgs,
			ToPushMsg: toPushMessage,
		}, nil
	}

	var count = uint64(0)
	var selectMsg []*types.Message
	var msgsErrInfo []msgErrInfo

	estimateMesssages := make([]*EstimateMessage, len(messages))
	for index, msg := range messages {
		// global msg meta
		newMsgMeta := messageSelector.messageMeta(msg.Meta)
		estimateMesssages[index] = &EstimateMessage{
			Msg: &msg.UnsignedMessage,
			Spec: &MessageSendSpec{
				MaxFee:            newMsgMeta.MaxFee,
				GasOverEstimation: newMsgMeta.GasOverEstimation,
			},
		}
		messageSelector.log.Debugf("estimate message %s meta maxfee %s, max fee cap %s, over estimation %f", msg.ID, newMsgMeta.MaxFee, newMsgMeta.MaxFeeCap, newMsgMeta.GasOverEstimation)
	}

	timeOutCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	estimateResult, err := messageSelector.nodeClient.GasBatchEstimateMessageGas(timeOutCtx, estimateMesssages, addr.Nonce, ts.Key())
	cancel()
	if err != nil {

		return nil, err
	}
	// sign
	for index, msg := range messages {
		//if error print error message
		if len(estimateResult[index].Err) != 0 {
			msgsErrInfo = append(msgsErrInfo, msgErrInfo{id: msg.ID, err: gasEstimate + estimateResult[index].Err})
			messageSelector.log.Errorf("estimate message %s fail %s", msg.ID, estimateResult[index].Err)
			continue
		}
		estimateMsg := estimateResult[index].Msg
		if count >= wantCount {
			break
		}
		addrInfo, ok := messageSelector.getAddrInfo(msg.WalletName, msg.From)
		if !ok {
			messageSelector.log.Warnf("not found wallet client %s", msg.WalletName)
			continue
		}
		if addrInfo.state != types.Alive && addrInfo.state != types.Forbiden {
			messageSelector.log.Infof("wallet %s address %v state is %s, skip select unchain message", msg.WalletName, addr.Addr, types.StateToString(addrInfo.state))
			continue
		}

		//分配nonce
		msg.Nonce = addr.Nonce
		msg.GasFeeCap = estimateMsg.GasFeeCap
		msg.GasPremium = estimateMsg.GasPremium
		msg.GasLimit = estimateMsg.GasLimit

		unsignedCid := msg.UnsignedMessage.Cid()
		msg.UnsignedCid = &unsignedCid
		//签名
		data, err := msg.UnsignedMessage.ToStorageBlock()
		if err != nil {
			messageSelector.log.Errorf("calc message unsigned message id %s fail %v", msg.ID, err)
			continue
		}

		timeOutCtx, cancel = context.WithTimeout(ctx, time.Second)
		sigI, err := handleTimeout(addrInfo.walletCli.WalletSign, timeOutCtx, []interface{}{addr.Addr, unsignedCid.Bytes(), core.MsgMeta{
			Type:  core.MTChainMsg,
			Extra: data.RawData(),
		}})
		cancel()
		if err != nil {
			//todo client net crash?
			msgsErrInfo = append(msgsErrInfo, msgErrInfo{id: msg.ID, err: signMsg + err.Error()})
			messageSelector.log.Errorf("wallet sign failed %s fail %v", msg.ID, err)
			continue
		}

		sig := sigI.(*crypto.Signature)
		msg.Signature = sig
		//state
		msg.State = types.FillMsg

		signedMsg := venusTypes.SignedMessage{
			Message:   msg.UnsignedMessage,
			Signature: *msg.Signature,
		}

		signedCid := signedMsg.Cid()
		msg.SignedCid = &signedCid

		selectMsg = append(selectMsg, msg)
		addr.Nonce++
		count++
	}

	messageSelector.log.Infof("address %s select message %d ExpireMsgs %d ToPushMsgs %d ErrMsgs %d max nonce %d",
		addr.Addr, len(selectMsg), len(expireMsgs), len(toPushMessage), len(msgsErrInfo), addr.Nonce)
	return &MsgSelectResult{
		SelectMsg: selectMsg,
		ExpireMsg: expireMsgs,
		ToPushMsg: toPushMessage,
		ErrMsg:    msgsErrInfo,
	}, nil
}

func (messageSelector *MessageSelector) excludeExpire(ts *venusTypes.TipSet, msgs []*types.Message) ([]*types.Message, []*types.Message) {
	//todo check whether message is expired
	var result []*types.Message
	var expireMsg []*types.Message
	for _, msg := range msgs {
		if msg.Meta.ExpireEpoch != 0 && msg.Meta.ExpireEpoch <= ts.Height() {
			//expire
			msg.State = types.FailedMsg
			expireMsg = append(expireMsg, msg)
			continue
		}
		result = append(result, msg)
	}
	return result, expireMsg
}

func (messageSelector *MessageSelector) messageMeta(meta *types.MsgMeta) *types.MsgMeta {
	newMsgMeta := &types.MsgMeta{}
	*newMsgMeta = *meta
	globalMeta := messageSelector.sps.GetParams().GetMsgMeta()

	if meta.GasOverEstimation == 0 {
		newMsgMeta.GasOverEstimation = globalMeta.GasOverEstimation
	}
	if meta.MaxFee.NilOrZero() {
		newMsgMeta.MaxFee = globalMeta.MaxFee
	}
	if meta.MaxFeeCap.NilOrZero() {
		newMsgMeta.MaxFeeCap = globalMeta.MaxFeeCap
	}

	return newMsgMeta
}

func (messageSelector *MessageSelector) getNonceInTipset(ctx context.Context, ts *venusTypes.TipSet) (*types.NonceMap, error) {
	applied := types.NewNonceMap()
	//todo change with venus/lotus message for tipset
	selectMsg := func(m *venusTypes.Message) error {
		// The first match for a sender is guaranteed to have correct nonce -- the block isn't valid otherwise
		if _, ok := applied.Get(m.From); !ok {
			applied.Add(m.From, m.Nonce)
		}

		val, _ := applied.Get(m.From)
		if val != m.Nonce {
			return nil
		}
		val++
		applied.Add(m.From, val)
		return nil
	}

	for _, b := range ts.Blocks() {
		fullBlk, err := messageSelector.nodeClient.ChainGetBlockMessages(ctx, b.Cid())
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		for _, bmsg := range fullBlk.BlsMessages {
			err := selectMsg(bmsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}

		}

		for _, smsg := range fullBlk.SecpkMessages {
			err := selectMsg(smsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}
		}
	}

	return applied, nil
}
func (messageSelector *MessageSelector) GasEstimateMessageGas(ctx context.Context, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, tsk venusTypes.TipSetKey) (*venusTypes.UnsignedMessage, error) {
	if msg.GasLimit == 0 {
		gasLimitI, err := handleTimeout(messageSelector.nodeClient.GasEstimateGasLimit, ctx, []interface{}{msg, venusTypes.EmptyTSK})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas used: %w", err)
		}
		gasLimit := gasLimitI.(int64)
		//GasOverEstimation default value should be 1.25
		msg.GasLimit = int64(float64(gasLimit) * meta.GasOverEstimation)
	}

	if msg.GasPremium == venusTypes.EmptyInt || venusTypes.BigCmp(msg.GasPremium, venusTypes.NewInt(0)) == 0 {
		gasPremiumI, err := handleTimeout(messageSelector.nodeClient.GasEstimateGasPremium, ctx, []interface{}{uint64(10), msg.From, msg.GasLimit, venusTypes.EmptyTSK})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas price: %w", err)
		}
		msg.GasPremium = gasPremiumI.(big.Int)
	}

	if msg.GasFeeCap == venusTypes.EmptyInt || venusTypes.BigCmp(msg.GasFeeCap, venusTypes.NewInt(0)) == 0 {
		feeCapI, err := handleTimeout(messageSelector.nodeClient.GasEstimateFeeCap, ctx, []interface{}{msg, int64(20), venusTypes.EmptyTSK})
		if err != nil {
			return nil, xerrors.Errorf("estimating fee cap: %w", err)
		}
		msg.GasFeeCap = feeCapI.(big.Int)
	}

	CapGasFee(msg, meta.MaxFee)

	return msg, nil
}

func (messageSelector *MessageSelector) clearAddrInfos() {
	for _, addrInfo := range messageSelector.addrInfos.addrInfos {
		if addrInfo.close != nil {
			addrInfo.close()
		}
	}
}

func (messageSelector *MessageSelector) getAddrInfo(walletName string, addr address.Address) (addrInfo, bool) {
	messageSelector.addrInfos.l.Lock()
	defer messageSelector.addrInfos.l.Unlock()

	key := walletName + ":" + addr.String()
	if info, ok := messageSelector.addrInfos.addrInfos[key]; ok {
		if info.err != nil {
			return addrInfo{}, false
		}
		return info, true
	}

	wa, err := messageSelector.walletService.GetWalletAddress(context.TODO(), walletName, addr)
	if err != nil {
		messageSelector.addrInfos.addrInfos[key] = addrInfo{err: err}
		return addrInfo{}, false
	}
	cli, close, err := messageSelector.walletService.GetWalletClient(context.TODO(), walletName)
	if err != nil {
		messageSelector.addrInfos.addrInfos[key] = addrInfo{err: err}
		return addrInfo{}, false
	}

	info := addrInfo{
		walletCli: &cli,
		close:     close,
		state:     wa.AddressState,
		err:       nil,
	}
	messageSelector.addrInfos.addrInfos[key] = info

	return info, true
}

func CapGasFee(msg *venusTypes.UnsignedMessage, maxFee abi.TokenAmount) {
	if maxFee.NilOrZero() {
		return
	}

	gl := venusTypes.NewInt(uint64(msg.GasLimit))
	totalFee := venusTypes.BigMul(msg.GasFeeCap, gl)

	if totalFee.LessThanEqual(maxFee) {
		return
	}

	msg.GasFeeCap = big.Div(maxFee, gl)
	msg.GasPremium = big.Min(msg.GasFeeCap, msg.GasPremium) // cap premium at FeeCap
}
