package service

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/filecoin-project/go-address"
	venustypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-messager/config"
	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

var ErrRecordExist = xerrors.Errorf("record exist")

type WalletService struct {
	repo           repo.Repo
	log            *logrus.Logger
	cfg            *config.WalletConfig
	sps            *SharedParamsService
	nodeClient     *NodeClient
	addressService *AddressService
	walletInfos    map[string]*WalletInfo

	pendingAddrChan chan pendingAddr
	walletDelChan   chan string

	wallets   map[string]types.UUID
	addresses map[address.Address]types.UUID

	l sync.RWMutex
}

type WalletInfo struct {
	walletCli    IWalletClient
	walletState  types.State
	addressInfos map[address.Address]*AddressInfo
}

type pendingAddr struct {
	walletName string
	addr       address.Address
}

func NewWalletService(repo repo.Repo,
	logger *logrus.Logger,
	nodeClient *NodeClient,
	addressService *AddressService,
	cfg *config.WalletConfig,
	sps *SharedParamsService) (*WalletService, error) {
	ws := &WalletService{
		repo:           repo,
		log:            logger,
		nodeClient:     nodeClient,
		addressService: addressService,
		cfg:            cfg,
		sps:            sps,

		walletDelChan:   make(chan string, 10),
		pendingAddrChan: make(chan pendingAddr, 10),
		walletInfos:     make(map[string]*WalletInfo),
		wallets:         make(map[string]types.UUID),
		addresses:       make(map[address.Address]types.UUID),
	}
	err := ws.start()

	return ws, err
}

func (walletService *WalletService) SaveWallet(ctx context.Context, wallet *types.Wallet) (types.UUID, error) {
	cli, _, err := newWalletClient(ctx, wallet.Url, wallet.Token)
	if err != nil {
		return types.UUID{}, err
	}
	if err := walletService.repo.Transaction(func(txRepo repo.TxRepo) error {
		has, err := txRepo.WalletRepo().HasWallet(wallet.Name)
		if err != nil {
			return err
		}
		if has {
			w, err := txRepo.WalletRepo().GetOneRecord(wallet.Name)
			if err != nil {
				return err
			}
			if w.IsDeleted == repo.NotDeleted && w.State == types.Alive {
				return ErrRecordExist
			}
			wallet.ID = w.ID
		}
		return txRepo.WalletRepo().SaveWallet(wallet)
	}); err != nil {
		return types.UUID{}, err
	}
	walletService.addWallet(wallet.Name, &WalletInfo{
		walletCli:    &cli,
		walletState:  wallet.State,
		addressInfos: make(map[address.Address]*AddressInfo),
	})
	walletService.setWallets(wallet.Name, wallet.ID)
	walletService.log.Infof("save wallet %v", wallet)

	return wallet.ID, nil
}

func (walletService *WalletService) GetWalletByID(ctx context.Context, uuid types.UUID) (*types.Wallet, error) {
	return walletService.repo.WalletRepo().GetWalletByID(uuid)
}

func (walletService *WalletService) GetWalletByName(ctx context.Context, name string) (*types.Wallet, error) {
	return walletService.repo.WalletRepo().GetWalletByName(name)
}

func (walletService *WalletService) HasWallet(ctx context.Context, name string) (bool, error) {
	return walletService.repo.WalletRepo().HasWallet(name)
}

func (walletService *WalletService) ListWallet(ctx context.Context) ([]*types.Wallet, error) {
	return walletService.repo.WalletRepo().ListWallet()
}

func (walletService *WalletService) ListRemoteWalletAddress(ctx context.Context, walletName string) ([]address.Address, error) {
	info, ok := walletService.getWalletInfo(walletName)
	if !ok {
		return nil, xerrors.Errorf("wallet %s not exit", walletName)
	}

	return info.walletCli.WalletList(ctx)
}

func (walletService *WalletService) DeleteWallet(ctx context.Context, name string) (string, error) {
	w, err := walletService.GetWalletByName(ctx, name)
	if err != nil {
		return "", err
	}

	if err := walletService.repo.WalletRepo().UpdateState(name, types.Removing); err != nil {
		return "", err
	}

	walletService.deleteWallet(w.Name)
	walletService.log.Infof("delete wallet %s", name)

	return name, nil
}

//// wallet address ////

func (walletService *WalletService) HasWalletAddress(ctx context.Context, walletName string, addr address.Address) (bool, error) {
	return walletService.repo.WalletAddressRepo().HasWalletAddress(walletService.getWalletID(walletName), walletService.getAddressID(addr))
}

func (walletService *WalletService) SetSelectMsgNum(ctx context.Context, walletName string, addr address.Address, num uint64) (address.Address, error) {
	if err := walletService.repo.WalletAddressRepo().UpdateSelectMsgNum(walletService.getWalletID(walletName), walletService.getAddressID(addr), num); err != nil {
		return addr, err
	}
	walletService.mutatorAddressInfo(walletName, addr, func(addressInfo *AddressInfo) {
		addressInfo.SelectMsgNum = num
	})

	return addr, nil
}

func (walletService *WalletService) ListWalletAddress(ctx context.Context) ([]*types.WalletAddress, error) {
	return walletService.repo.WalletAddressRepo().ListWalletAddress()
}

func (walletService *WalletService) GetWalletAddress(ctx context.Context, walletName string, addr address.Address) (*types.WalletAddress, error) {
	return walletService.repo.WalletAddressRepo().GetWalletAddress(walletService.getWalletID(walletName), walletService.getAddressID(addr))
}

func (walletService *WalletService) start() error {
	// load local address
	addrList, err := walletService.repo.AddressRepo().ListAddress(context.TODO())
	if err != nil {
		return err
	}
	for _, addr := range addrList {
		walletService.setAddresses(addr.Addr, addr.ID)
	}

	// load local wallet
	walletList, err := walletService.ListWallet(context.TODO())
	if err != nil {
		return err
	}
	for _, w := range walletList {
		cli, _, err := newWalletClient(context.Background(), w.Url, w.Token)
		if err != nil {
			return err
		}
		addressInfos := make(map[address.Address]*AddressInfo)
		if waList, err := walletService.repo.WalletAddressRepo().GetWalletAddressByWalletID(w.ID); err == nil {
			for _, wa := range waList {
				if wa.AddressState == types.Removing { // maybe need push signed message to mpool
					addr := walletService.getAddressByAddrID(wa.AddrID)
					addressInfos[addr] = &AddressInfo{
						State:        wa.AddressState,
						SelectMsgNum: wa.SelMsgNum,
						WalletClient: &cli,
					}
				}
			}
		}
		walletService.addWallet(w.Name, &WalletInfo{
			walletCli:    &cli,
			walletState:  w.State,
			addressInfos: addressInfos,
		})
		walletService.setWallets(w.Name, w.ID)
	}

	// scan remote address
	for walletName, cli := range walletService.ListWalletClient() {
		if err := walletService.ProcessWallet(context.TODO(), walletName, cli); err != nil {
			walletService.log.Errorf("process wallet failed %v %v", walletName, err)
		}
	}

	go walletService.listenWalletChange(context.TODO())
	go walletService.checkWalletState()
	go walletService.checkAddressState()

	return nil
}

func (walletService *WalletService) listenWalletChange(ctx context.Context) {
	interval := time.Duration(walletService.cfg.ScanInterval) * time.Second
	params := walletService.sps.GetParams()
	if params.SharedParams != nil && params.ScanInterval != 0 {
		interval = time.Duration(params.ScanInterval) * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			walletList, err := walletService.ListWallet(ctx)
			if err != nil {
				walletService.log.Errorf("list wallet %v", err)
				continue
			}
			var cli IWalletClient
			walletClis := walletService.ListWalletClient()
			for _, w := range walletList {
				// maybe add a new wallet
				if _, ok := walletClis[w.Name]; !ok {
					cliT, _, err := newWalletClient(ctx, w.Url, w.Token)
					if err != nil {
						walletService.log.Errorf("new wallet client %v", err)
						continue
					}
					cli = &cliT
					walletService.addWallet(w.Name, &WalletInfo{
						walletCli:    &cliT,
						walletState:  types.Alive,
						addressInfos: make(map[address.Address]*AddressInfo),
					})
					walletService.setWallets(w.Name, w.ID)
				} else {
					cli = walletClis[w.Name]
					delete(walletClis, w.Name)
				}
				if err := walletService.ProcessWallet(ctx, w.Name, cli); err != nil {
					walletService.log.Errorf("process wallet failed %v %v", w.Name, err)
				}
			}

			// delete the corresponding wallet in the cache when db delete a wallet
			for walletName := range walletClis {
				walletService.deleteWallet(walletName)
			}
		case i := <-walletService.sps.GetParams().ScanIntervalChan:
			ticker.Reset(i)
		case <-ctx.Done():
			walletService.log.Warnf("context error: %v", ctx.Err())
			return
		}
	}
}

func (walletService *WalletService) ProcessWallet(ctx context.Context, walletName string, cli IWalletClient) error {
	addrList, err := cli.WalletList(ctx)
	if err != nil {
		return err
	}

	walletAddrs := walletService.listOneWalletAddress(walletName)
	for _, addr := range addrList {
		delete(walletAddrs, addr)

		if addrInfo, ok := walletService.GetAddressInfo(walletName, addr); ok &&
			(addrInfo.State == types.Alive || addrInfo.State == types.Forbiden) {
			continue
		}
		// store address
		if err := walletService.saveAddress(ctx, addr); err != nil {
			walletService.log.Errorf("save address %v", err)
			continue
		}

		if err := walletService.updateWalletAddress(ctx, cli, walletName, addr); err != nil {
			walletService.log.Errorf("save wallet address %v", err)
			continue
		}
		walletService.log.Infof("wallet %s add address %s", walletName, addr.String())
	}

	// address to handle remote wallet deletion
	for addr := range walletAddrs {
		addrInfo, ok := walletService.GetAddressInfo(walletName, addr)
		if !ok || addrInfo.State == types.Removing {
			continue
		}
		walletService.delAddress(walletName, addr)
	}

	return nil
}

// update address table
func (walletService *WalletService) saveAddress(ctx context.Context, addr address.Address) error {
	var nonce uint64
	actor, err := walletService.nodeClient.StateGetActor(context.Background(), addr, venustypes.EmptyTSK)
	if err != nil {
		walletService.log.Infof("get actor failed, addr: %s, err: %v", addr, err)
	} else {
		nonce = actor.Nonce //current nonce should big than nonce on chain
	}

	addrTmp := &types.Address{
		ID:        types.NewUUID(),
		Addr:      addr,
		Nonce:     nonce,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
		IsDeleted: -1,
	}
	addrID, err := walletService.addressService.SaveAddress(ctx, addrTmp)
	if err != nil && !xerrors.Is(err, ErrRecordExist) {
		return err
	}
	walletService.setAddresses(addr, addrID)
	return nil
}

// update wallet address table
func (walletService *WalletService) updateWalletAddress(ctx context.Context, cli IWalletClient, walletName string, addr address.Address) error {
	walletID := walletService.getWalletID(walletName)
	addrID := walletService.getAddressID(addr)

	wa := &types.WalletAddress{
		ID:           types.NewUUID(),
		WalletID:     walletID,
		AddrID:       addrID,
		AddressState: types.Alive,
		IsDeleted:    -1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	var selMsgNum uint64
	state := types.Alive

	if err := walletService.repo.Transaction(func(txRepo repo.TxRepo) error {
		has, err := txRepo.WalletAddressRepo().HasWalletAddress(walletID, addrID)
		if err != nil {
			return err
		}
		if has {
			walletAddress, err := txRepo.WalletAddressRepo().GetOneRecord(walletID, addrID)
			if err != nil && xerrors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if walletAddress.SelMsgNum != 0 { // inherit SelMsgNum
				wa.SelMsgNum = walletAddress.SelMsgNum
				selMsgNum = walletAddress.SelMsgNum
			}
			wa.CreatedAt = walletAddress.CreatedAt
			wa.ID = walletAddress.ID
			if walletAddress.AddressState == types.Forbiden { // keep Forbiden status
				wa.AddressState = walletAddress.AddressState
				state = walletAddress.AddressState
			}
		}
		return txRepo.WalletAddressRepo().SaveWalletAddress(wa)
	}); err != nil {
		return err
	}

	// update cache
	walletService.mutatorAddressInfo(walletName, addr, func(addressInfo *AddressInfo) {
		*addressInfo = AddressInfo{
			State:        state,
			SelectMsgNum: selMsgNum,
			WalletClient: cli,
		}
	})

	return nil
}

func (walletService *WalletService) checkWalletState() {
	walletList, err := walletService.repo.WalletRepo().ListWallet()
	if err != nil {
		walletService.log.Errorf("got wallet %v", err)
	}
	for _, w := range walletList {
		if w.State == types.Removing {
			walletService.walletDelChan <- w.Name
		}
	}
	for walletName := range walletService.walletDelChan {
		checkAgain := true
		if walletInfo, ok := walletService.getWalletInfo(walletName); !ok || walletInfo.walletState == types.Alive {
			checkAgain = false
		}

		addrs := walletService.listOneWalletAddress(walletName)
		if len(addrs) == 0 {
			if err := walletService.repo.WalletRepo().DelWallet(walletName); err != nil && !xerrors.Is(err, gorm.ErrRecordNotFound) {
				walletService.log.Errorf("deleted wallet(%s) %v", walletName, err)
			} else {
				walletService.removeWallet(walletName)
				walletService.log.Infof("deleted wallet %s", walletName)
				checkAgain = false
			}
		}
		if checkAgain {
			go func() {
				time.Sleep(time.Second * 30)
				walletService.walletDelChan <- walletName
			}()
		}
	}
}

func (walletService *WalletService) checkAddressState() {
	walletAddrList, err := walletService.repo.WalletAddressRepo().ListWalletAddress()
	if err != nil {
		walletService.log.Errorf("get wallet address %v", err)
	}

	for _, wa := range walletAddrList {
		if wa.AddressState == types.Removing {
			walletService.pendingAddrChan <- pendingAddr{walletName: walletService.getWalletName(wa.WalletID),
				addr: walletService.getAddressByAddrID(wa.AddrID)}
		}
	}

	for target := range walletService.pendingAddrChan {
		var isDeleted bool
		msgs, err := walletService.repo.MessageRepo().ListFilledMessageByWallet(target.walletName, target.addr)
		if err != nil {
			walletService.log.Errorf("got filled message %v", err)
		} else if len(msgs) == 0 {
			// add address again
			if addrInfo, ok := walletService.GetAddressInfo(target.walletName, target.addr); ok && addrInfo.State == types.Alive {
				isDeleted = true
			} else {
				if err := walletService.repo.WalletAddressRepo().DelWalletAddress(walletService.getWalletID(target.walletName),
					walletService.getAddressID(target.addr)); err != nil {
					walletService.log.Errorf("delete wallet address %v", err)
				}
				walletService.removeAddressInfo(target.walletName, target.addr)
				walletService.log.Infof("deleted address %v", target.addr.String())

				// not using address, delete it
				if _, ok := walletService.AllAddresses()[target.addr]; !ok {
					if _, err = walletService.addressService.DeleteAddress(context.TODO(), target.addr); err != nil {
						walletService.log.Errorf("delete address %v", err)
					}
				}
				isDeleted = true
			}
		}
		if !isDeleted {
			go func() {
				time.Sleep(time.Second * 30)
				walletService.pendingAddrChan <- target
			}()
		}
	}
}

func (walletService *WalletService) ForbiddenAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error) {
	if err := walletService.repo.WalletAddressRepo().UpdateAddressState(walletService.getWalletID(walletName),
		walletService.getAddressID(addr), types.Forbiden); err != nil {
		return address.Undef, err
	}

	walletService.mutatorAddressInfo(walletName, addr, func(addressInfo *AddressInfo) {
		addressInfo.State = types.Forbiden
	})
	walletService.log.Infof("forbidden address %v", addr.String())

	return addr, nil
}

func (walletService *WalletService) ActiveAddress(ctx context.Context, walletName string, addr address.Address) (address.Address, error) {
	if err := walletService.repo.WalletAddressRepo().UpdateAddressState(walletService.getWalletID(walletName),
		walletService.getAddressID(addr), types.Alive); err != nil {
		return address.Undef, err
	}

	walletService.mutatorAddressInfo(walletName, addr, func(addressInfo *AddressInfo) {
		addressInfo.State = types.Alive
	})
	walletService.log.Infof("active address %v", addr.String())

	return addr, nil
}

/// wallet info ///

func (walletService *WalletService) addWallet(walletName string, walletInfo *WalletInfo) {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	walletService.walletInfos[walletName] = walletInfo
}

func (walletService *WalletService) getWalletInfo(walletName string) (*WalletInfo, bool) {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	walletInfo, ok := walletService.walletInfos[walletName]

	return walletInfo, ok
}

func (walletService *WalletService) GetAddressInfo(walletName string, addr address.Address) (AddressInfo, bool) {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	if walletInfo, ok := walletService.walletInfos[walletName]; ok {
		if addrInfo, ok := walletInfo.addressInfos[addr]; ok && addrInfo != nil {
			return *addrInfo, ok
		}
	}

	return AddressInfo{}, false
}

func (walletService *WalletService) GetAddressesInfo(addr address.Address) (map[string]AddressInfo, bool) {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	addrsInfo := make(map[string]AddressInfo)
	for walletName, walletInfo := range walletService.walletInfos {
		if addrInfo, ok := walletInfo.addressInfos[addr]; ok && addrInfo != nil {
			addrsInfo[walletName] = *addrInfo
		}
	}

	return addrsInfo, len(addrsInfo) > 0
}

func (walletService *WalletService) HasAddress(walletName string, addr address.Address) bool {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	if walletInfo, ok := walletService.walletInfos[walletName]; ok {
		if addrInfo, ok := walletInfo.addressInfos[addr]; ok && addrInfo.State == types.Alive {
			return true
		}
	}

	return false
}

func (walletService *WalletService) ListWalletClient() map[string]IWalletClient {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	clis := make(map[string]IWalletClient, len(walletService.walletInfos))
	for walletName, info := range walletService.walletInfos {
		if info.walletState != types.Alive {
			continue
		}
		clis[walletName] = info.walletCli
	}

	return clis
}

func (walletService *WalletService) listOneWalletAddress(walletName string) map[address.Address]struct{} {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	var addrs map[address.Address]struct{}
	if walletInfo, ok := walletService.walletInfos[walletName]; ok {
		addrs = make(map[address.Address]struct{}, len(walletInfo.addressInfos))
		for addr := range walletInfo.addressInfos {
			addrs[addr] = struct{}{}
		}
	}

	return addrs
}

func (walletService *WalletService) AllAddresses() map[address.Address]struct{} {
	walletService.l.RLock()
	defer walletService.l.RUnlock()
	addrs := make(map[address.Address]struct{})
	for _, walletInfo := range walletService.walletInfos {
		for addr := range walletInfo.addressInfos {
			addrs[addr] = struct{}{}
		}
	}

	return addrs
}

func (walletService *WalletService) mutatorAddressInfo(walletName string, addr address.Address, f func(addressInfo *AddressInfo)) {
	walletService.l.Lock()
	defer walletService.l.Unlock()
	if walletInfo, ok := walletService.walletInfos[walletName]; ok {
		if addrInfo, ok := walletInfo.addressInfos[addr]; ok {
			f(addrInfo)
		} else {
			walletInfo.addressInfos[addr] = &AddressInfo{}
			f(walletInfo.addressInfos[addr])
		}
	}
}

func (walletService *WalletService) deleteWallet(walletName string) {
	walletService.l.Lock()
	var addrs []address.Address
	if info, ok := walletService.walletInfos[walletName]; ok {
		info.walletState = types.Removing
		walletService.walletDelChan <- walletName
		for addr := range info.addressInfos {
			addrs = append(addrs, addr)
		}
	}
	walletService.l.Unlock()

	for _, addr := range addrs {
		walletService.delAddress(walletName, addr)
	}
}

func (walletService *WalletService) removeWallet(walletName string) {
	walletService.l.Lock()
	defer walletService.l.Unlock()
	delete(walletService.walletInfos, walletName)
}

func (walletService *WalletService) delAddress(walletName string, addr address.Address) {
	walletID := walletService.getWalletID(walletName)
	addrID := walletService.getAddressID(addr)

	walletService.l.Lock()
	defer walletService.l.Unlock()
	walletInfo, ok := walletService.walletInfos[walletName]
	if !ok {
		return
	}
	if addrInfo, ok := walletInfo.addressInfos[addr]; ok {
		addrInfo.State = types.Removing
		if err := walletService.repo.WalletAddressRepo().UpdateAddressState(walletID, addrID, types.Removing); err != nil {
			walletService.log.Errorf("update wallet address state %v", err)
		}
		if err := walletService.repo.MessageRepo().UpdateUnFilledMessageState(walletName, addr, types.NoWalletMsg); err != nil {
			walletService.log.Errorf("update unfilled message state %v", err)
		}
		go func() {
			walletService.pendingAddrChan <- pendingAddr{walletName: walletName, addr: addr}
		}()
	}
	walletService.log.Infof("wallet delete address %s", addr.String())
}

func (walletService *WalletService) removeAddressInfo(walletName string, addr address.Address) {
	walletService.l.Lock()
	defer walletService.l.Unlock()
	if walletInfo, ok := walletService.walletInfos[walletName]; ok {
		delete(walletInfo.addressInfos, addr)
	}
}

//// wallet name and wallet id mapping
func (walletService *WalletService) setWallets(walletName string, walletID types.UUID) {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	walletService.wallets[walletName] = walletID
}

func (walletService *WalletService) getWalletID(walletName string) types.UUID {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	return walletService.wallets[walletName]
}

func (walletService *WalletService) getWalletName(walletID types.UUID) string {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	for k, v := range walletService.wallets {
		if v == walletID {
			return k
		}
	}

	return ""
}

//// address and address id mapping
func (walletService *WalletService) setAddresses(addr address.Address, addrID types.UUID) {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	walletService.addresses[addr] = addrID
}

func (walletService *WalletService) getAddressID(addr address.Address) types.UUID {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	return walletService.addresses[addr]
}

func (walletService *WalletService) getAddressByAddrID(addrID types.UUID) address.Address {
	walletService.l.Lock()
	defer walletService.l.Unlock()

	for k, v := range walletService.addresses {
		if v == addrID {
			return k
		}
	}

	return address.Undef
}
