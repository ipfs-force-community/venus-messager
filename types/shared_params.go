package types

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

type SharedParams struct {
	ID uint `json:"id"`

	ExpireEpoch       abi.ChainEpoch `json:"expireEpoch"`
	GasOverEstimation float64        `json:"gasOverEstimation"`
	MaxFee            big.Int        `json:"maxFee,omitempty"`
	MaxFeeCap         big.Int        `json:"maxFeeCap"`

	SelMsgNum uint64 `json:"selMsgNum"`

	ScanInterval int `json:"scanInterval"` // second

	MaxEstFailNumOfMsg uint64 `json:"maxEstFailNumOfMsg"`
}

func (sp *SharedParams) GetMsgMeta() *MsgMeta {
	if sp == nil {
		return nil
	}
	return &MsgMeta{
		ExpireEpoch:       sp.ExpireEpoch,
		GasOverEstimation: sp.GasOverEstimation,
		MaxFee:            sp.MaxFee,
		MaxFeeCap:         sp.MaxFeeCap,
	}
}
