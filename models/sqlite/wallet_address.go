package sqlite

import (
	"fmt"
	"reflect"
	"time"

	"github.com/hunjixin/automapper"
	"gorm.io/gorm"

	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

type sqliteWalletAddress struct {
	ID           types.UUID  `gorm:"column:id;type:varchar(256);primary_key"`
	WalletID     types.UUID  `gorm:"column:wallet_id;type:varchar(256);NOT NULL"`
	AddrID       types.UUID  `gorm:"column:addr_id;type:varchar(256);NOT NULL"`
	AddressState types.State `gorm:"column:addr_state;type:int;index:wallet_addr_state;"`
	SelMsgNum    uint64      `gorm:"column:sel_msg_num;type:unsigned bigint;NOT NULL"`

	IsDeleted int       `gorm:"column:is_deleted;index;default:-1;NOT NULL"` // 是否删除 1:是  -1:否
	CreatedAt time.Time `gorm:"column:created_at;index;NOT NULL"`            // 创建时间
	UpdatedAt time.Time `gorm:"column:updated_at;index;NOT NULL"`            // 更新时间
}

func FromWalletAddress(walletAddr types.WalletAddress) *sqliteWalletAddress {
	return automapper.MustMapper(&walletAddr, TSqliteWalletAddress).(*sqliteWalletAddress)
}

func (sqliteWalletAddress sqliteWalletAddress) WalletAddress() *types.WalletAddress {
	return automapper.MustMapper(&sqliteWalletAddress, TWalletAddress).(*types.WalletAddress)
}

func (sqliteWalletAddress sqliteWalletAddress) TableName() string {
	return "wallet_addresses"
}

var _ repo.WalletAddressRepo = (*sqliteWalletAddressRepo)(nil)

type sqliteWalletAddressRepo struct {
	*gorm.DB
}

func newSqliteWalletAddressRepo(db *gorm.DB) sqliteWalletAddressRepo {
	return sqliteWalletAddressRepo{DB: db}
}

func (s sqliteWalletAddressRepo) SaveWalletAddress(wa *types.WalletAddress) error {
	sqliteWalletAddress := FromWalletAddress(*wa)
	sqliteWalletAddress.UpdatedAt = time.Now()
	return s.DB.Save(sqliteWalletAddress).Error
}

func (s sqliteWalletAddressRepo) GetWalletAddress(walletID, addrID types.UUID) (*types.WalletAddress, error) {
	var wa sqliteWalletAddress
	if err := s.DB.Where("wallet_id = ? and addr_id = ? and is_deleted = -1", walletID, addrID).
		First(&wa).Error; err != nil {
		return nil, err
	}
	fmt.Println(wa)
	return wa.WalletAddress(), nil
}

func (s sqliteWalletAddressRepo) GetOneRecord(walletID, addrID types.UUID) (*types.WalletAddress, error) {
	var wa sqliteWalletAddress
	if err := s.DB.Where("wallet_id = ? and addr_id = ?", walletID, addrID).First(&wa).Error; err != nil {
		return nil, err
	}
	return wa.WalletAddress(), nil
}

func (s sqliteWalletAddressRepo) GetWalletAddressByWalletID(walletID types.UUID) ([]*types.WalletAddress, error) {
	var internalWalletAddress []*sqliteWalletAddress
	if err := s.DB.Find(&internalWalletAddress, "wallet_id = ? and is_deleted = ?", walletID, -1).Error; err != nil {
		return nil, err
	}

	result, err := automapper.Mapper(internalWalletAddress, reflect.TypeOf([]*types.WalletAddress{}))
	if err != nil {
		return nil, err
	}

	return result.([]*types.WalletAddress), nil
}

func (s sqliteWalletAddressRepo) HasWalletAddress(walletID, addrID types.UUID) (bool, error) {
	var count int64
	if err := s.DB.Model(&sqliteWalletAddress{}).Where("wallet_id = ? and addr_id = ?", walletID, addrID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s sqliteWalletAddressRepo) ListWalletAddress() ([]*types.WalletAddress, error) {
	var internalWalletAddress []*sqliteWalletAddress
	if err := s.DB.Find(&internalWalletAddress, "is_deleted = ?", -1).Error; err != nil {
		return nil, err
	}

	result, err := automapper.Mapper(internalWalletAddress, reflect.TypeOf([]*types.WalletAddress{}))
	if err != nil {
		return nil, err
	}

	return result.([]*types.WalletAddress), nil
}

func (s sqliteWalletAddressRepo) UpdateAddressState(walletID, addrID types.UUID, state types.State) error {
	return s.DB.Model((*sqliteWalletAddress)(nil)).Where("wallet_id = ? and addr_id = ?", walletID, addrID).
		UpdateColumn("addr_state", state).Error
}

func (s sqliteWalletAddressRepo) UpdateSelectMsgNum(walletID, addrID types.UUID, selMsgNum uint64) error {
	return s.DB.Model((*sqliteWalletAddress)(nil)).Where("wallet_id = ? and addr_id = ?", walletID, addrID).
		UpdateColumn("sel_msg_num", selMsgNum).Error
}

func (s sqliteWalletAddressRepo) DelWalletAddress(walletID, addrID types.UUID) error {
	var wa sqliteWalletAddress
	if err := s.DB.Where("wallet_id = ? and addr_id = ? and is_deleted = -1", walletID, addrID).
		First(&wa).Error; err != nil {
		return err
	}
	wa.IsDeleted = 1
	wa.AddressState = types.Removed

	return s.DB.Save(&wa).Error
}