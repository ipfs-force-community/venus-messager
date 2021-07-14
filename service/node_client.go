package service

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus-messager/config"
	"github.com/filecoin-project/venus/app/submodule/apitypes"
	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"
	"github.com/ipfs/go-cid"
)

type EstimateMessage struct {
	Msg  *types.Message
	Spec *types.MessageSendSpec
}

type EstimateResult struct {
	Msg *types.Message
	Err string
}

type NodeClient struct {
	ChainNotify              func(context.Context) (<-chan []*chain.HeadChange, error)
	ChainHead                func(context.Context) (*types.TipSet, error)
	ChainGetTipSet           func(context.Context, types.TipSetKey) (*types.TipSet, error)
	ChainGetBlock            func(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainGetBlockMessages    func(context.Context, cid.Cid) (*apitypes.BlockMessages, error)
	ChainGetMessagesInTipset func(context.Context, types.TipSetKey) ([]apitypes.Message, error)
	ChainGetParentMessages   func(ctx context.Context, bcid cid.Cid) ([]apitypes.Message, error)
	ChainGetParentReceipts   func(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	StateAccountKey          func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateSearchMsg           func(context.Context, cid.Cid) (*chain.MsgLookup, error)
	StateGetActor            func(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)
	StateLookupID            func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)

	GasEstimateMessageGas      func(context.Context, *types.UnsignedMessage, *types.MessageSendSpec, types.TipSetKey) (*types.UnsignedMessage, error)
	GasEstimateFeeCap          func(context.Context, *types.UnsignedMessage, int64, types.TipSetKey) (big.Int, error)
	GasEstimateGasPremium      func(context.Context, uint64, address.Address, int64, types.TipSetKey) (big.Int, error)
	GasEstimateGasLimit        func(ctx context.Context, msgIn *types.UnsignedMessage, tsk types.TipSetKey) (int64, error)
	GasBatchEstimateMessageGas func(ctx context.Context, estimateMessages []*EstimateMessage, fromNonce uint64, tsk types.TipSetKey) ([]*EstimateResult, error)

	MpoolPush      func(context.Context, *types.SignedMessage) (cid.Cid, error)
	MpoolBatchPush func(context.Context, []*types.SignedMessage) ([]cid.Cid, error)

	//broadcast interface
	MpoolPublishByAddr  func(ctx context.Context, addr address.Address) error
	MpoolPublishMessage func(ctx context.Context, smsg *types.SignedMessage) error
}

func NewNodeClient(ctx context.Context, cfg *config.NodeConfig) (*NodeClient, jsonrpc.ClientCloser, error) {
	apiInfo := apiinfo.NewAPIInfo(cfg.Url, cfg.Token)
	addr, err := apiInfo.DialArgs("v0")
	if err != nil {
		return nil, nil, err
	}
	var res NodeClient
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin", []interface{}{&res}, apiInfo.AuthHeader())
	return &res, closer, err
}
