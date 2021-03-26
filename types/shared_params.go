package types

import (
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

type SharedParams struct {
	ID uint `json:"id"`

	ExpireEpoch       abi.ChainEpoch `json:"expireEpoch"`
	GasOverEstimation float64        `json:"gasOverEstimation"`
	MaxFee            int64          `json:"maxFee,omitempty"`
	MaxFeeCap         int64          `json:"maxFeeCap"`

	SelMsgNum uint64 `json:"selMsgNum"`

	ScanInterval time.Duration `json:"scanInterval"`

	MaxEstFailNumOfMsg uint64 `json:"maxEstFailNumOfMsg"`
}

func (sp *SharedParams) GetMsgMeta() *MsgMeta {
	return &MsgMeta{
		ExpireEpoch:       sp.ExpireEpoch,
		GasOverEstimation: sp.GasOverEstimation,
		MaxFee:            big.NewInt(sp.MaxFee),
		MaxFeeCap:         big.NewInt(sp.MaxFeeCap),
	}
}