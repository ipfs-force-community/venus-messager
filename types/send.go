package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

const (
	ParamsJSON = "json"
	ParamsHex  = "hex"
)

type SendParams struct {
	To      address.Address
	From    address.Address
	Val     abi.TokenAmount
	Account string

	GasPremium *abi.TokenAmount
	GasFeeCap  *abi.TokenAmount
	GasLimit   *int64

	Method     abi.MethodNum
	Params     string
	ParamsType string // json or hex
}
