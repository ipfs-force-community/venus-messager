package mysql

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"gorm.io/gorm"

	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
)

type mysqlAddress struct {
	ID                types.UUID `gorm:"column:id;type:varchar(256);primary_key"`
	Addr              string     `gorm:"column:addr;type:varchar(256);NOT NULL"` // 主键
	Nonce             uint64     `gorm:"column:nonce;type:bigint unsigned;index;NOT NULL"`
	Weight            int64      `gorm:"column:weight;type:bigint;index;NOT NULL"`
	GasOverEstimation float64    `gorm:"column:gas_over_estimation;type:decimal(10,2);"`
	MaxFee            types.Int  `gorm:"column:max_fee;type:varchar(256);"`
	MaxFeeCap         types.Int  `gorm:"column:max_fee_cap;type:varchar(256);"`

	IsDeleted int       `gorm:"column:is_deleted;index;default:-1;NOT NULL"` // 是否删除 1:是  -1:否
	CreatedAt time.Time `gorm:"column:created_at;index;NOT NULL"`            // 创建时间
	UpdatedAt time.Time `gorm:"column:updated_at;index;NOT NULL"`            // 更新时间
}

func (s mysqlAddress) TableName() string {
	return "addresses"
}

func FromAddress(addr *types.Address) *mysqlAddress {
	sqliteAddr := &mysqlAddress{
		ID:        addr.ID,
		Addr:      addr.Addr.String(),
		Nonce:     addr.Nonce,
		Weight:    addr.Weight,
		IsDeleted: addr.IsDeleted,
		CreatedAt: addr.CreatedAt,
		UpdatedAt: addr.UpdatedAt,
	}
	if !addr.MaxFee.Nil() {
		sqliteAddr.MaxFee = types.NewFromGo(addr.MaxFee.Int)
	}
	if !addr.MaxFeeCap.Nil() {
		sqliteAddr.MaxFeeCap = types.NewFromGo(addr.MaxFeeCap.Int)
	}

	return sqliteAddr
}

func (s mysqlAddress) Address() (*types.Address, error) {
	addr, err := address.NewFromString(s.Addr)
	if err != nil {
		return nil, err
	}
	return &types.Address{
		ID:                s.ID,
		Addr:              addr,
		Nonce:             s.Nonce,
		Weight:            s.Weight,
		GasOverEstimation: s.GasOverEstimation,
		MaxFee:            big.Int{Int: s.MaxFee.Int},
		MaxFeeCap:         big.Int{Int: s.MaxFeeCap.Int},
		IsDeleted:         s.IsDeleted,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}, nil
}

type mysqlAddressRepo struct {
	*gorm.DB
}

var _ repo.AddressRepo = &mysqlAddressRepo{}

func newMysqlAddressRepo(db *gorm.DB) *mysqlAddressRepo {
	return &mysqlAddressRepo{DB: db}
}

func (s mysqlAddressRepo) SaveAddress(ctx context.Context, addr *types.Address) error {
	return s.DB.Save(FromAddress(addr)).Error
}

func (s mysqlAddressRepo) UpdateAddress(ctx context.Context, addr *types.Address) error {
	updateColumns := map[string]interface{}{
		"nonce":      addr.Nonce,
		"is_deleted": addr.IsDeleted,
	}
	return s.DB.Model(&mysqlAddress{}).Where("addr = ?", addr.Addr.String()).Updates(updateColumns).Error
}

func (s mysqlAddressRepo) UpdateNonce(ctx context.Context, addr address.Address, nonce uint64) error {
	return s.DB.Model(&mysqlAddress{}).Where("addr = ?", addr.String()).
		UpdateColumn("nonce", nonce).Error
}

func (s mysqlAddressRepo) HasAddress(ctx context.Context, addr address.Address) (bool, error) {
	var count int64
	err := s.DB.Model(&mysqlAddress{}).Where("addr = ?", addr.String()).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s mysqlAddressRepo) GetAddress(ctx context.Context, addr address.Address) (*types.Address, error) {
	var a mysqlAddress
	if err := s.DB.Take(&a, "addr = ? and is_deleted = -1", addr.String()).Error; err != nil {
		return nil, err
	}

	return a.Address()
}

func (s mysqlAddressRepo) GetAddressByID(ctx context.Context, id types.UUID) (*types.Address, error) {
	var a mysqlAddress
	if err := s.DB.Where("id = ? and is_deleted = -1", id).First(&a).Error; err != nil {
		return nil, err
	}

	return a.Address()
}

func (s mysqlAddressRepo) GetOneRecord(ctx context.Context, addr address.Address) (*types.Address, error) {
	var a mysqlAddress
	if err := s.DB.Take(&a, "addr = ?", addr.String()).Error; err != nil {
		return nil, err
	}

	return a.Address()
}

func (s mysqlAddressRepo) DelAddress(ctx context.Context, addr address.Address) error {
	return s.DB.Model((*mysqlAddress)(nil)).Where("addr = ? and is_deleted = -1", addr.String()).
		Updates(map[string]interface{}{"is_deleted": 1}).Error
}

func (s mysqlAddressRepo) ListAddress(ctx context.Context) ([]*types.Address, error) {
	var list []*mysqlAddress
	if err := s.DB.Find(&list, "is_deleted = ?", -1).Error; err != nil {
		return nil, err
	}

	result := make([]*types.Address, len(list))
	for index, r := range list {
		addr, err := r.Address()
		if err != nil {
			return nil, err
		}
		result[index] = addr
	}

	return result, nil
}
