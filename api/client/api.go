package client

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-messager/types"
)

type IMessager interface {
	HasMessageByUid(ctx context.Context, id string) (bool, error)                                                                                  //perm:read
	WaitMessage(ctx context.Context, id string, confidence uint64) (*types.Message, error)                                                         //perm:read
	PushMessage(ctx context.Context, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error)                      //perm:write
	PushMessageWithId(ctx context.Context, id string, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error)     //perm:write
	GetMessageByUid(ctx context.Context, id string) (*types.Message, error)                                                                        //perm:read
	GetMessageByCid(ctx context.Context, id cid.Cid) (*types.Message, error)                                                                       //perm:read
	GetMessageBySignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error)                                                                //perm:read
	GetMessageByUnsignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error)                                                              //perm:read
	GetMessageByFromAndNonce(ctx context.Context, from address.Address, nonce uint64) (*types.Message, error)                                      //perm:read
	ListMessage(ctx context.Context) ([]*types.Message, error)                                                                                     //perm:admin
	ListMessageByAddress(ctx context.Context, addr address.Address) ([]*types.Message, error)                                                      //perm:admin
	ListFailedMessage(ctx context.Context) ([]*types.Message, error)                                                                               //perm:admin
	ListBlockedMessage(ctx context.Context, addr address.Address, d time.Duration) ([]*types.Message, error)                                       //perm:admin
	UpdateMessageStateByCid(ctx context.Context, cid cid.Cid, state types.MessageState) (cid.Cid, error)                                           //perm:admin
	UpdateMessageStateByID(ctx context.Context, id string, state types.MessageState) (string, error)                                               //perm:admin
	UpdateAllFilledMessage(ctx context.Context) (int, error)                                                                                       //perm:admin
	UpdateFilledMessageByID(ctx context.Context, id string) (string, error)                                                                        //perm:admin
	ReplaceMessage(ctx context.Context, id string, auto bool, maxFee string, gasLimit int64, gasPremium string, gasFeecap string) (cid.Cid, error) //perm:admin
	RepublishMessage(ctx context.Context, id string) (struct{}, error)                                                                             //perm:admin
	MarkBadMessage(ctx context.Context, id string) (struct{}, error)                                                                               //perm:admin

	SaveWallet(ctx context.Context, wallet *types.Wallet) (types.UUID, error)            //perm:admin
	GetWalletByName(ctx context.Context, name string) (*types.Wallet, error)             //perm:admin
	GetWalletByID(ctx context.Context, id types.UUID) (*types.Wallet, error)             //perm:admin
	HasWallet(ctx context.Context, name string) (bool, error)                            //perm:admin
	ListWallet(ctx context.Context) ([]*types.Wallet, error)                             //perm:admin
	ListRemoteWalletAddress(ctx context.Context, name string) ([]address.Address, error) //perm:admin
	DeleteWallet(ctx context.Context, name string) (string, error)                       //perm:admin
	UpdateWallet(ctx context.Context, wallet *types.Wallet) (string, error)              //perm:admin

	SaveAddress(ctx context.Context, address *types.Address) (string, error)                      //perm:admin
	GetAddress(ctx context.Context, addr address.Address) (*types.Address, error)                 //perm:admin
	HasAddress(ctx context.Context, addr address.Address) (bool, error)                           //perm:admin
	ListAddress(ctx context.Context) ([]*types.Address, error)                                    //perm:admin
	UpdateNonce(ctx context.Context, addr address.Address, nonce uint64) (address.Address, error) //perm:admin
	DeleteAddress(ctx context.Context, addr address.Address) (address.Address, error)             //perm:admin

	GetSharedParams(ctx context.Context) (*types.SharedParams, error)                             //perm:admin
	SetSharedParams(ctx context.Context, params *types.SharedParams) (*types.SharedParams, error) //perm:admin
	RefreshSharedParams(ctx context.Context) (struct{}, error)                                    //perm:admin

	SaveNode(ctx context.Context, node *types.Node) (struct{}, error) //perm:admin
	GetNode(ctx context.Context, name string) (*types.Node, error)    //perm:admin
	HasNode(ctx context.Context, name string) (bool, error)           //perm:admin
	ListNode(ctx context.Context) ([]*types.Node, error)              //perm:admin
	DeleteNode(ctx context.Context, name string) (struct{}, error)    //perm:admin

	GetWalletAddress(ctx context.Context, walletName string, addr address.Address) (*types.WalletAddress, error)       //perm:admin
	ForbiddenAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error)            //perm:admin
	ActiveAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error)               //perm:admin
	SetSelectMsgNum(ctx context.Context, walletName string, addr address.Address, num uint64) (address.Address, error) //perm:admin
	HasWalletAddress(ctx context.Context, walletName string, addr address.Address) (bool, error)                       //perm:read
	ListWalletAddress(ctx context.Context) ([]*types.WalletAddress, error)                                             //perm:admin
}

var _ IMessager = (*Message)(nil)

type Message struct {
	Internal struct {
		HasMessageByUid          func(ctx context.Context, id string) (bool, error)
		WaitMessage              func(ctx context.Context, id string, confidence uint64) (*types.Message, error)
		PushMessage              func(ctx context.Context, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error)
		PushMessageWithId        func(ctx context.Context, id string, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error)
		GetMessageByUid          func(ctx context.Context, id string) (*types.Message, error)
		GetMessageByCid          func(ctx context.Context, id cid.Cid) (*types.Message, error)
		GetMessageBySignedCid    func(ctx context.Context, cid cid.Cid) (*types.Message, error)
		GetMessageByUnsignedCid  func(ctx context.Context, cid cid.Cid) (*types.Message, error)
		GetMessageByFromAndNonce func(ctx context.Context, from address.Address, nonce uint64) (*types.Message, error)
		ListMessage              func(ctx context.Context) ([]*types.Message, error)
		ListMessageByAddress     func(ctx context.Context, addr address.Address) ([]*types.Message, error)
		ListFailedMessage        func(ctx context.Context) ([]*types.Message, error)
		ListBlockedMessage       func(ctx context.Context, addr address.Address, d time.Duration) ([]*types.Message, error)
		UpdateMessageStateByCid  func(ctx context.Context, cid cid.Cid, state types.MessageState) (cid.Cid, error)
		UpdateMessageStateByID   func(ctx context.Context, id string, state types.MessageState) (string, error)
		UpdateAllFilledMessage   func(ctx context.Context) (int, error)
		UpdateFilledMessageByID  func(ctx context.Context, id string) (string, error)
		ReplaceMessage           func(ctx context.Context, id string, auto bool, maxFee string, gasLimit int64, gasPremium string, gasFeecap string) (cid.Cid, error)
		RepublishMessage         func(ctx context.Context, id string) (struct{}, error)
		MarkBadMessage           func(ctx context.Context, id string) (struct{}, error)

		SaveWallet              func(ctx context.Context, wallet *types.Wallet) (types.UUID, error)
		GetWalletByName         func(ctx context.Context, name string) (*types.Wallet, error)
		GetWalletByID           func(ctx context.Context, id types.UUID) (*types.Wallet, error)
		HasWallet               func(ctx context.Context, name string) (bool, error)
		ListWallet              func(ctx context.Context) ([]*types.Wallet, error)
		ListRemoteWalletAddress func(ctx context.Context, name string) ([]address.Address, error)
		DeleteWallet            func(ctx context.Context, name string) (string, error)
		UpdateWallet            func(ctx context.Context, wallet *types.Wallet) (string, error)

		SaveAddress   func(ctx context.Context, address *types.Address) (string, error)
		GetAddress    func(ctx context.Context, addr address.Address) (*types.Address, error)
		HasAddress    func(ctx context.Context, addr address.Address) (bool, error)
		ListAddress   func(ctx context.Context) ([]*types.Address, error)
		UpdateNonce   func(ctx context.Context, addr address.Address, nonce uint64) (address.Address, error)
		DeleteAddress func(ctx context.Context, addr address.Address) (address.Address, error)

		GetSharedParams     func(context.Context) (*types.SharedParams, error)
		SetSharedParams     func(context.Context, *types.SharedParams) (*types.SharedParams, error)
		RefreshSharedParams func(ctx context.Context) (struct{}, error)

		SaveNode   func(ctx context.Context, node *types.Node) (struct{}, error)
		GetNode    func(ctx context.Context, name string) (*types.Node, error)
		HasNode    func(ctx context.Context, name string) (bool, error)
		ListNode   func(ctx context.Context) ([]*types.Node, error)
		DeleteNode func(ctx context.Context, name string) (struct{}, error)

		GetWalletAddress  func(ctx context.Context, walletName string, addr address.Address) (*types.WalletAddress, error)
		ForbiddenAddress  func(ctx context.Context, walletName string, addr address.Address) (address.Address, error)
		ActiveAddress     func(ctx context.Context, walletName string, addr address.Address) (address.Address, error)
		SetSelectMsgNum   func(ctx context.Context, walletName string, addr address.Address, num uint64) (address.Address, error)
		HasWalletAddress  func(ctx context.Context, walletName string, addr address.Address) (bool, error)
		ListWalletAddress func(ctx context.Context) ([]*types.WalletAddress, error)
	}
}

func (message *Message) HasMessageByUid(ctx context.Context, id string) (bool, error) {
	return message.Internal.HasMessageByUid(ctx, id)
}

func (message *Message) PushMessage(ctx context.Context, msg *venusTypes.Message, meta *types.MsgMeta, walletName string) (string, error) {
	return message.Internal.PushMessage(ctx, msg, meta, walletName)
}

func (message *Message) PushMessageWithId(ctx context.Context, id string, msg *venusTypes.UnsignedMessage, meta *types.MsgMeta, walletName string) (string, error) {
	return message.Internal.PushMessageWithId(ctx, id, msg, meta, walletName)
}

func (message *Message) GetMessageByUid(ctx context.Context, id string) (*types.Message, error) {
	return message.Internal.GetMessageByUid(ctx, id)
}

func (message *Message) GetMessageByCid(ctx context.Context, id cid.Cid) (*types.Message, error) {
	return message.Internal.GetMessageByCid(ctx, id)
}

func (message *Message) GetMessageByUnsignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error) {
	return message.Internal.GetMessageByUnsignedCid(ctx, cid)
}

func (message *Message) GetMessageBySignedCid(ctx context.Context, cid cid.Cid) (*types.Message, error) {
	return message.Internal.GetMessageBySignedCid(ctx, cid)
}

func (message *Message) GetMessageByFromAndNonce(ctx context.Context, from address.Address, nonce uint64) (*types.Message, error) {
	return message.Internal.GetMessageByFromAndNonce(ctx, from, nonce)
}

func (message *Message) ListMessage(ctx context.Context) ([]*types.Message, error) {
	return message.Internal.ListMessage(ctx)
}

func (message *Message) ListMessageByAddress(ctx context.Context, addr address.Address) ([]*types.Message, error) {
	return message.Internal.ListMessageByAddress(ctx, addr)
}

func (message *Message) ListFailedMessage(ctx context.Context) ([]*types.Message, error) {
	return message.Internal.ListFailedMessage(ctx)
}

func (message *Message) ListBlockedMessage(ctx context.Context, addr address.Address, d time.Duration) ([]*types.Message, error) {
	return message.Internal.ListBlockedMessage(ctx, addr, d)
}

func (message *Message) UpdateMessageStateByCid(ctx context.Context, cid cid.Cid, state types.MessageState) (cid.Cid, error) {
	return message.Internal.UpdateMessageStateByCid(ctx, cid, state)
}

func (message *Message) UpdateMessageStateByID(ctx context.Context, id string, state types.MessageState) (string, error) {
	return message.Internal.UpdateMessageStateByID(ctx, id, state)
}

func (message *Message) UpdateAllFilledMessage(ctx context.Context) (int, error) {
	return message.Internal.UpdateAllFilledMessage(ctx)
}

func (message *Message) UpdateFilledMessageByID(ctx context.Context, id string) (string, error) {
	return message.Internal.UpdateFilledMessageByID(ctx, id)
}

func (message *Message) ReplaceMessage(ctx context.Context, id string, auto bool, maxFee string, gasLimit int64, gasPremium string, gasFeecap string) (cid.Cid, error) {
	return message.Internal.ReplaceMessage(ctx, id, auto, maxFee, gasLimit, gasPremium, gasFeecap)
}

func (message *Message) RepublishMessage(ctx context.Context, id string) (struct{}, error) {
	return message.Internal.RepublishMessage(ctx, id)
}

func (message *Message) MarkBadMessage(ctx context.Context, id string) (struct{}, error) {
	return message.Internal.MarkBadMessage(ctx, id)
}

func (message *Message) WaitMessage(ctx context.Context, id string, confidence uint64) (*types.Message, error) {
	tm := time.NewTicker(time.Second * 30)
	defer tm.Stop()

	doneCh := make(chan struct{}, 1)
	doneCh <- struct{}{}

	for {
		select {
		case <-doneCh:
			msg, err := message.Internal.GetMessageByUid(ctx, id)
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

///////  wallet  ///////

func (message *Message) SaveWallet(ctx context.Context, wallet *types.Wallet) (types.UUID, error) {
	return message.Internal.SaveWallet(ctx, wallet)
}

func (message *Message) GetWalletByName(ctx context.Context, name string) (*types.Wallet, error) {
	return message.Internal.GetWalletByName(ctx, name)
}

func (message *Message) GetWalletByID(ctx context.Context, id types.UUID) (*types.Wallet, error) {
	return message.Internal.GetWalletByID(ctx, id)
}

func (message *Message) HasWallet(ctx context.Context, name string) (bool, error) {
	return message.Internal.HasWallet(ctx, name)
}

func (message *Message) ListRemoteWalletAddress(ctx context.Context, name string) ([]address.Address, error) {
	return message.Internal.ListRemoteWalletAddress(ctx, name)
}

func (message *Message) ListWallet(ctx context.Context) ([]*types.Wallet, error) {
	return message.Internal.ListWallet(ctx)
}

func (message *Message) DeleteWallet(ctx context.Context, name string) (string, error) {
	return message.Internal.DeleteWallet(ctx, name)
}

func (message *Message) UpdateWallet(ctx context.Context, wallet *types.Wallet) (string, error) {
	return message.Internal.UpdateWallet(ctx, wallet)
}

///////  address ///////

func (message *Message) SaveAddress(ctx context.Context, address *types.Address) (string, error) {
	return message.Internal.SaveAddress(ctx, address)
}

func (message *Message) GetAddress(ctx context.Context, addr address.Address) (*types.Address, error) {
	return message.Internal.GetAddress(ctx, addr)
}

func (message *Message) HasAddress(ctx context.Context, addr address.Address) (bool, error) {
	return message.Internal.HasAddress(ctx, addr)
}

func (message *Message) ListAddress(ctx context.Context) ([]*types.Address, error) {
	return message.Internal.ListAddress(ctx)
}

func (message *Message) UpdateNonce(ctx context.Context, addr address.Address, nonce uint64) (address.Address, error) {
	return message.Internal.UpdateNonce(ctx, addr, nonce)
}

func (message *Message) DeleteAddress(ctx context.Context, addr address.Address) (address.Address, error) {
	return message.Internal.DeleteAddress(ctx, addr)
}

/////// shared params ///////

func (message *Message) GetSharedParams(ctx context.Context) (*types.SharedParams, error) {
	return message.Internal.GetSharedParams(ctx)
}

func (message *Message) SetSharedParams(ctx context.Context, params *types.SharedParams) (*types.SharedParams, error) {
	return message.Internal.SetSharedParams(ctx, params)
}

func (message *Message) RefreshSharedParams(ctx context.Context) (struct{}, error) {
	return message.Internal.RefreshSharedParams(ctx)
}

/////// node info ///////

func (message *Message) SaveNode(ctx context.Context, node *types.Node) (struct{}, error) {
	return message.Internal.SaveNode(ctx, node)
}

func (message *Message) GetNode(ctx context.Context, name string) (*types.Node, error) {
	return message.Internal.GetNode(ctx, name)
}

func (message *Message) HasNode(ctx context.Context, name string) (bool, error) {
	return message.Internal.HasNode(ctx, name)
}

func (message *Message) ListNode(ctx context.Context) ([]*types.Node, error) {
	return message.Internal.ListNode(ctx)
}

func (message *Message) DeleteNode(ctx context.Context, name string) (struct{}, error) {
	return message.Internal.DeleteNode(ctx, name)
}

/////// wallet address ///////

func (message *Message) ForbiddenAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error) {
	return message.Internal.ForbiddenAddress(ctx, walletName, addr)
}

func (message *Message) ActiveAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error) {
	return message.Internal.ActiveAddress(ctx, walletName, addr)
}

func (message *Message) SetSelectMsgNum(ctx context.Context, walletName string, addr address.Address, num uint64) (address.Address, error) {
	return message.Internal.SetSelectMsgNum(ctx, walletName, addr, num)
}

func (message *Message) HasWalletAddress(ctx context.Context, walletName string, addr address.Address) (bool, error) {
	return message.Internal.HasWalletAddress(ctx, walletName, addr)
}

func (message *Message) ListWalletAddress(ctx context.Context) ([]*types.WalletAddress, error) {
	return message.Internal.ListWalletAddress(ctx)
}

func (message *Message) GetWalletAddress(ctx context.Context, walletName string, addr address.Address) (*types.WalletAddress, error) {
	return message.Internal.GetWalletAddress(ctx, walletName, addr)
}
