package service

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"golang.org/x/xerrors"
	"gorm.io/gorm"

	"github.com/filecoin-project/venus-messager/gateway"
	"github.com/filecoin-project/venus-messager/log"
	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
)

var errAddressNotExists = xerrors.New("address not exists")

type AddressService struct {
	repo repo.Repo
	log  *log.Logger

	sps          *SharedParamsService
	nodeClient   *NodeClient
	walletClient *gateway.IWalletCli

	resetAddressFunc chan func() (uint64, error)
	resetAddressRes  chan resetAddressResult
}

func NewAddressService(repo repo.Repo,
	logger *log.Logger,
	sps *SharedParamsService,
	walletClient *gateway.IWalletCli,
	nodeClient *NodeClient) *AddressService {
	addressService := &AddressService{
		repo: repo,
		log:  logger,

		sps:          sps,
		nodeClient:   nodeClient,
		walletClient: walletClient,

		resetAddressFunc: make(chan func() (uint64, error)),
		resetAddressRes:  make(chan resetAddressResult),
	}

	return addressService
}

func (addressService *AddressService) SaveAddress(ctx context.Context, address *types.Address) (types.UUID, error) {
	err := addressService.repo.Transaction(func(txRepo repo.TxRepo) error {
		has, err := txRepo.AddressRepo().HasAddress(ctx, address.Addr)
		if err != nil {
			return err
		}
		if has {
			return xerrors.Errorf("address already exists")
		}
		return txRepo.AddressRepo().SaveAddress(ctx, address)
	})

	return address.ID, err
}

func (addressService *AddressService) UpdateNonce(ctx context.Context, addr address.Address, nonce uint64) (address.Address, error) {
	return addr, addressService.repo.AddressRepo().UpdateNonce(ctx, addr, nonce)
}

func (addressService *AddressService) GetAddress(ctx context.Context, addr address.Address) (*types.Address, error) {
	return addressService.repo.AddressRepo().GetAddress(ctx, addr)
}

func (addressService *AddressService) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	_, account := ipAccountFromContext(ctx)
	return addressService.walletClient.WalletHas(ctx, account, addr)
}

func (addressService *AddressService) HasAddress(ctx context.Context, addr address.Address) (bool, error) {
	return addressService.repo.AddressRepo().HasAddress(ctx, addr)
}

func (addressService *AddressService) ListAddress(ctx context.Context) ([]*types.Address, error) {
	return addressService.repo.AddressRepo().ListAddress(ctx)
}

func (addressService *AddressService) DeleteAddress(ctx context.Context, addr address.Address) (address.Address, error) {
	return addr, addressService.repo.AddressRepo().DelAddress(ctx, addr)
}

func (addressService *AddressService) ForbiddenAddress(ctx context.Context, addr address.Address) (address.Address, error) {
	if err := addressService.repo.AddressRepo().UpdateState(ctx, addr, types.Forbiden); err != nil {
		return address.Undef, err
	}
	addressService.log.Infof("forbidden address %v success", addr.String())

	return addr, nil
}

func (addressService *AddressService) ActiveAddress(ctx context.Context, addr address.Address) (address.Address, error) {
	if err := addressService.repo.AddressRepo().UpdateState(ctx, addr, types.Alive); err != nil {
		return address.Undef, err
	}
	addressService.log.Infof("active address %v success", addr.String())

	return addr, nil
}

func (addressService *AddressService) SetSelectMsgNum(ctx context.Context, addr address.Address, num uint64) (address.Address, error) {
	if err := addressService.repo.AddressRepo().UpdateSelectMsgNum(ctx, addr, num); err != nil {
		return addr, err
	}
	addressService.log.Infof("set select msg num: %s %d", addr.String(), num)

	return addr, nil
}

func (addressService *AddressService) SetFeeParams(ctx context.Context, addr address.Address, gasOverEstimation float64, maxFeeStr, maxFeeCapStr string) (address.Address, error) {
	has, err := addressService.repo.AddressRepo().HasAddress(ctx, addr)
	if err != nil {
		return address.Undef, err
	}
	if !has {
		return address.Undef, errAddressNotExists
	}

	var needUpdate bool
	var maxFee, maxFeeCap big.Int
	if len(maxFeeStr) != 0 {
		maxFee, err = venusTypes.BigFromString(maxFeeStr)
		if err != nil {
			return address.Undef, xerrors.Errorf("parsing max-spend: %v", err)
		}
		needUpdate = true
	}
	if len(maxFeeCapStr) != 0 {
		maxFeeCap, err = venusTypes.BigFromString(maxFeeCapStr)
		if err != nil {
			return address.Undef, xerrors.Errorf("parsing max-feecap: %v", err)
		}
		needUpdate = true
	}
	if !needUpdate && gasOverEstimation == 0 {
		return addr, nil
	}

	return addr, addressService.repo.AddressRepo().UpdateFeeParams(ctx, addr, gasOverEstimation, maxFee, maxFeeCap)
}

type resetAddressResult struct {
	latestNonce uint64
	err         error
}

func (addressService *AddressService) resetAddress(ctx context.Context, addr address.Address, targetNonce uint64) (uint64, error) {
	addrInfo, err := addressService.GetAddress(ctx, addr)
	if err != nil {
		return 0, err
	}
	actor, err := addressService.nodeClient.StateGetActor(ctx, addr, venusTypes.EmptyTSK)
	if err != nil {
		return 0, err
	}

	if targetNonce != 0 {
		if targetNonce < actor.Nonce {
			return 0, xerrors.Errorf("target nonce(%d) smaller than chain nonce(%d)", targetNonce, actor.Nonce)
		}
	} else {
		targetNonce = actor.Nonce
	}
	addressService.log.Infof("reset address target nonce %d, chain nonce %d", targetNonce, actor.Nonce)

	latestNonce := addrInfo.Nonce
	if err := addressService.repo.Transaction(func(txRepo repo.TxRepo) error {
		for nonce := addrInfo.Nonce - 1; nonce >= targetNonce; nonce-- {
			msg, err := txRepo.MessageRepo().GetMessageByFromNonceAndState(addr, nonce, types.FillMsg)
			if err != nil {
				if xerrors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return xerrors.Errorf("found message by address(%s) and nonce(%d) failed %v", addr.String(), nonce, err)
			}
			if msg.State == types.FillMsg {
				if _, err := txRepo.MessageRepo().MarkBadMessage(msg.ID); err != nil {
					return xerrors.Errorf("mark bad message %s failed %v", msg.ID, err)
				}
				latestNonce = nonce
			} else if msg.State == types.OnChainMsg {
				break
			}
		}

		unFillMsgs, err := txRepo.MessageRepo().ListUnFilledMessage(addr)
		if err != nil {
			return err
		}
		for _, msg := range unFillMsgs {
			if _, err := txRepo.MessageRepo().MarkBadMessage(msg.ID); err != nil {
				return xerrors.Errorf("mark bad message %s failed %v", msg.ID, err)
			}
		}

		if latestNonce < addrInfo.Nonce {
			return txRepo.AddressRepo().UpdateNonce(ctx, addr, latestNonce)
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return latestNonce, nil
}

func (addressService *AddressService) ResetAddress(ctx context.Context, addr address.Address, targetNonce uint64) (uint64, error) {
	addressService.resetAddressFunc <- func() (uint64, error) {
		return addressService.resetAddress(ctx, addr, targetNonce)
	}

	select {
	case r, ok := <-addressService.resetAddressRes:
		if !ok {
			return 0, xerrors.Errorf("unexpect error")
		}
		addressService.log.Infof("reset address %s success, current nonce %d ", addr.String(), r.latestNonce)
		return r.latestNonce, r.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (addressService *AddressService) Addresses() map[address.Address]struct{} {
	addrs := make(map[address.Address]struct{})
	addrList, err := addressService.ListAddress(context.Background())
	if err != nil {
		addressService.log.Errorf("list address %v", err)
		return addrs
	}

	for _, addr := range addrList {
		addrs[addr.Addr] = struct{}{}
	}

	return addrs
}
