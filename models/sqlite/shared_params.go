package sqlite

import (
	"context"
	"github.com/filecoin-project/go-state-types/big"

	"gorm.io/gorm"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
)

type sqliteSharedParams struct {
	ID uint `gorm:"primary_key;column:id;type:INT unsigned AUTO_INCREMENT;NOT NULL" json:"id"`

	ExpireEpoch       abi.ChainEpoch `gorm:"column:expire_epoch;type:INT;NOT NULL"`
	GasOverEstimation float64        `gorm:"column:gas_over_estimation;type:REAL;NOT NULL"`
	MaxFee            types.Int      `gorm:"column:max_fee;type:varchar(256);NOT NULL"`
	MaxFeeCap         types.Int      `gorm:"column:max_fee_cap;type:varchar(256);NOT NULL"`

	SelMsgNum uint64 `gorm:"column:sel_msg_num;type:UNSIGNED BIG INT;NOT NULL"`

	ScanInterval int `gorm:"column:scan_interval;NOT NULL"`

	MaxEstFailNumOfMsg uint64 `gorm:"column:max_ext_fail_num_of_msg;type:UNSIGNED BIG INT;NOT NULL"`
}

func FromSharedParams(sp types.SharedParams) *sqliteSharedParams {
	return &sqliteSharedParams{
		ID:                 sp.ID,
		ExpireEpoch:        sp.ExpireEpoch,
		GasOverEstimation:  sp.GasOverEstimation,
		MaxFee:             types.Int{sp.MaxFee.Int},
		MaxFeeCap:          types.Int{sp.MaxFeeCap.Int},
		SelMsgNum:          sp.SelMsgNum,
		ScanInterval:       sp.ScanInterval,
		MaxEstFailNumOfMsg: sp.MaxEstFailNumOfMsg,
	}
}

func (ssp sqliteSharedParams) SharedParams() *types.SharedParams {
	return &types.SharedParams{
		ID:                 ssp.ID,
		ExpireEpoch:        ssp.ExpireEpoch,
		GasOverEstimation:  ssp.GasOverEstimation,
		MaxFee:             big.NewFromGo(ssp.MaxFee.Int),
		MaxFeeCap:          big.NewFromGo(ssp.MaxFeeCap.Int),
		SelMsgNum:          ssp.SelMsgNum,
		ScanInterval:       ssp.ScanInterval,
		MaxEstFailNumOfMsg: ssp.MaxEstFailNumOfMsg,
	}
}

func (ssp sqliteSharedParams) TableName() string {
	return "shared_params"
}

var _ repo.SharedParamsRepo = (*sqliteSharedParamsRepo)(nil)

type sqliteSharedParamsRepo struct {
	*gorm.DB
}

func newSqliteSharedParamsRepo(db *gorm.DB) sqliteSharedParamsRepo {
	return sqliteSharedParamsRepo{DB: db}
}

func (s sqliteSharedParamsRepo) GetSharedParams(ctx context.Context) (*types.SharedParams, error) {
	var ssp sqliteSharedParams
	if err := s.DB.Take(&ssp).Error; err != nil {
		return nil, err
	}
	return ssp.SharedParams(), nil
}

func (s sqliteSharedParamsRepo) SetSharedParams(ctx context.Context, params *types.SharedParams) (uint, error) {
	var ssp sqliteSharedParams
	if err := s.DB.Where("id = ?", 1).Take(&ssp).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			if params.ID == 0 {
				params.ID = 1
			}
			if err := s.DB.Save(FromSharedParams(*params)).Error; err != nil {
				return 0, err
			}
			return params.ID, nil
		}
		return 0, err
	}

	ssp.ExpireEpoch = params.ExpireEpoch
	ssp.GasOverEstimation = params.GasOverEstimation
	ssp.MaxFeeCap = types.Int{params.MaxFeeCap.Int}
	ssp.MaxFee = types.Int{params.MaxFee.Int}

	ssp.SelMsgNum = params.SelMsgNum

	ssp.ScanInterval = params.ScanInterval

	ssp.MaxEstFailNumOfMsg = params.MaxEstFailNumOfMsg

	if err := s.DB.Save(&ssp).Error; err != nil {
		return 0, err
	}

	return params.ID, nil
}
