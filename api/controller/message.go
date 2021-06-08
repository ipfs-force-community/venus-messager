package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus-messager/service"
	"github.com/filecoin-project/venus-messager/types"
)

type Message struct {
	BaseController
	MsgService *service.MessageService
}

func (message Message) PushMessage(ctx context.Context, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error) {
	newId := types.NewUUID()
	if msg.Method == 5 {
		fmt.Println("xxxxxxxxx origin gas fee cap ", msg.GasFeeCap)
	}
	err := message.MsgService.PushMessage(ctx, &types.Message{
		ID:              newId.String(),
		UnsignedMessage: *msg,
		Meta:            meta,
		State:           types.UnFillMsg,
		WalletName:      walletName,
	})

	return newId.String(), err
}

func (message Message) PushMessageWithId(ctx context.Context, id string, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error) {
	if msg.Method == 5 {
		fmt.Println("xxxxxxxxx origin gas fee cap ", msg.GasFeeCap)
	}
	return id, message.MsgService.PushMessage(ctx, &types.Message{
		ID:              id,
		UnsignedMessage: *msg,
		Meta:            meta,
		State:           types.UnFillMsg,
		WalletName:      walletName,
	})
}

func (message Message) HasMessageByUid(ctx context.Context, id string) (bool, error) {
	return message.MsgService.HasMessageByUid(ctx, id)
}

func (message Message) GetMessageByUid(ctx context.Context, id string) (*types.Message, error) {
	return message.MsgService.GetMessageByUid(ctx, id)
}

func (message Message) GetMessageByCid(ctx context.Context, id cid.Cid) (*types.Message, error) {
	return message.MsgService.GetMessageByCid(ctx, id)
}

func (message Message) GetMessageState(ctx context.Context, id string) (types.MessageState, error) {
	return message.MsgService.GetMessageState(ctx, id)
}

func (message Message) GetMessageBySignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error) {
	return message.MsgService.GetMessageBySignedCid(ctx, cid)
}

func (message Message) GetMessageByUnsignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error) {
	return message.MsgService.GetMessageByUnsignedCid(ctx, cid)
}

func (message Message) GetMessageByFromAndNonce(ctx context.Context, from address.Address, nonce uint64) (*types.Message, error) {
	return message.MsgService.GetMessageByFromAndNonce(ctx, from, nonce)
}

func (message Message) ListMessage(ctx context.Context) ([]*types.Message, error) {
	return message.MsgService.ListMessage(ctx)
}

func (message Message) ListMessageByFromState(ctx context.Context, from address.Address, state types.MessageState, pageIndex, pageSize int) ([]*types.Message, error) {
	return message.MsgService.ListMessageByFromState(ctx, from, state, pageIndex, pageSize)
}

func (message Message) ListMessageByAddress(ctx context.Context, addr address.Address) ([]*types.Message, error) {
	return message.MsgService.ListMessageByAddress(ctx, addr)
}

func (message Message) ListFailedMessage(ctx context.Context) ([]*types.Message, error) {
	return message.MsgService.ListFailedMessage(ctx)
}

func (message Message) ListBlockedMessage(ctx context.Context, addr address.Address, d time.Duration) ([]*types.Message, error) {
	return message.MsgService.ListBlockedMessage(ctx, addr, d)
}

func (message Message) UpdateMessageStateByCid(ctx context.Context, cid string, state types.MessageState) (string, error) {
	return message.MsgService.UpdateMessageStateByCid(ctx, cid, state)
}

func (message Message) UpdateMessageStateByID(ctx context.Context, id string, state types.MessageState) (string, error) {
	return message.MsgService.UpdateMessageStateByID(ctx, id, state)
}

func (message Message) UpdateAllFilledMessage(ctx context.Context) (int, error) {
	return message.MsgService.UpdateAllFilledMessage(ctx)
}

func (message Message) UpdateFilledMessageByID(ctx context.Context, id string) (string, error) {
	return message.MsgService.UpdateSignedMessageByID(ctx, id)
}

func (message Message) ReplaceMessage(ctx context.Context, id string, auto bool, maxFee string, gasLimit int64, gasPremium string, gasFeecap string) (cid.Cid, error) {
	return message.MsgService.ReplaceMessage(ctx, id, auto, maxFee, gasLimit, gasPremium, gasFeecap)
}

func (message Message) RepublishMessage(ctx context.Context, id string) (struct{}, error) {
	return message.MsgService.RepublishMessage(ctx, id)
}

func (message Message) MarkBadMessage(ctx context.Context, id string) (struct{}, error) {
	return message.MsgService.MarkBadMessage(ctx, id)
}
