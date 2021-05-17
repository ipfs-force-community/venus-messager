package types

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
)

type Address struct {
	ID   UUID            `json:"id"`
	Addr address.Address `json:"addr"`
	//max for current, use nonce and +1
	Nonce  uint64 `json:"nonce"`
	Weight int64  `json:"weight"`

	GasOverEstimation float64 `json:"gasOverEstimation"`
	MaxFee            big.Int `json:"maxFee,omitempty"`
	MaxFeeCap         big.Int `json:"maxFeeCap"`

	IsDeleted int       `json:"isDeleted"`
	CreatedAt time.Time `json:"createAt"`
	UpdatedAt time.Time `json:"updateAt"`
}

type FeeParams struct {
}
