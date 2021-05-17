package cli

import (
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/go-address"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var AddrCmds = &cli.Command{
	Name:  "address",
	Usage: "address commands",
	Subcommands: []*cli.Command{
		searchAddrCmd,
		listAddrCmd,
		updateNonceCmd,
		setFeeParamsCmd,
	},
}

var searchAddrCmd = &cli.Command{
	Name:      "search",
	Usage:     "search address",
	ArgsUsage: "address",
	Action: func(ctx *cli.Context) error {
		client, closer, err := getAPI(ctx)
		if err != nil {
			return err
		}
		defer closer()

		if !ctx.Args().Present() {
			return xerrors.Errorf("must pass address")
		}

		addr, err := address.NewFromString(ctx.Args().First())
		if err != nil {
			return err
		}
		addrInfo, err := client.GetAddress(ctx.Context, addr)
		if err != nil {
			return err
		}
		bytes, err := json.MarshalIndent(addrInfo, " ", "\t")
		if err != nil {
			return err
		}
		fmt.Println(string(bytes))
		return nil
	},
}

var listAddrCmd = &cli.Command{
	Name:  "list",
	Usage: "list address",
	Action: func(ctx *cli.Context) error {
		client, closer, err := getAPI(ctx)
		if err != nil {
			return err
		}
		defer closer()

		addrs, err := client.ListAddress(ctx.Context)
		if err != nil {
			return err
		}

		bytes, err := json.MarshalIndent(addrs, " ", "\t")
		if err != nil {
			return err
		}
		fmt.Println(string(bytes))
		return nil
	},
}

var updateNonceCmd = &cli.Command{
	Name:      "update-nonce",
	Usage:     "update address nonce",
	ArgsUsage: "address",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "nonce",
			Usage: "address nonce",
		},
	},
	Action: func(ctx *cli.Context) error {
		client, closer, err := getAPI(ctx)
		if err != nil {
			return err
		}
		defer closer()

		if !ctx.Args().Present() {
			return xerrors.Errorf("must pass address")
		}

		addr, err := address.NewFromString(ctx.Args().First())
		if err != nil {
			return err
		}

		nonce := ctx.Uint64("nonce")
		if _, err := client.UpdateNonce(ctx.Context, addr, nonce); err != nil {
			return err
		}

		return nil
	},
}

// nolint
var deleteAddrCmd = &cli.Command{
	Name:      "del",
	Usage:     "delete local address",
	ArgsUsage: "address",
	Action: func(ctx *cli.Context) error {
		client, closer, err := getAPI(ctx)
		if err != nil {
			return err
		}
		defer closer()

		if !ctx.Args().Present() {
			return xerrors.Errorf("must pass address")
		}

		addr, err := address.NewFromString(ctx.Args().First())
		if err != nil {
			return err
		}
		_, err = client.DeleteAddress(ctx.Context, addr)
		if err != nil {
			return err
		}

		return nil
	},
}

var setFeeParamsCmd = &cli.Command{
	Name:      "set-fee-params",
	Usage:     "Address setting fee associated configuration",
	ArgsUsage: "address",
	Flags: []cli.Flag{
		&cli.Float64Flag{
			Name:  "gas-overestimation",
			Usage: "Estimate the coefficient of gas",
		},
		&cli.StringFlag{
			Name:  "max-feecap",
			Usage: "Max feecap for a message (burn and pay to miner, attoFIL/GasUnit)",
		},
		&cli.StringFlag{
			Name:  "max-fee",
			Usage: "Spend up to X attoFIL for message",
		},
	},
	Action: func(ctx *cli.Context) error {
		client, closer, err := getAPI(ctx)
		if err != nil {
			return err
		}
		defer closer()

		if !ctx.Args().Present() {
			return xerrors.Errorf("must pass address")
		}

		addr, err := address.NewFromString(ctx.Args().First())
		if err != nil {
			return err
		}
		addrInfo, err := client.GetAddress(ctx.Context, addr)
		if err != nil {
			return err
		}

		if ctx.IsSet("gas-overestimation") {
			addrInfo.GasOverEstimation = ctx.Float64("gas-overestimation")
		}
		if ctx.IsSet("max-fee") {
			addrInfo.MaxFee, err = venusTypes.BigFromString(ctx.String("max-fee"))
			if err != nil {
				return xerrors.Errorf("parsing max-spend: %w", err)
			}
		}
		if ctx.IsSet("max-feecap") {
			addrInfo.MaxFeeCap, err = venusTypes.BigFromString(ctx.String("max-feecap"))
			if err != nil {
				return xerrors.Errorf("parsing max-feecap: %w", err)
			}
		}

		_, err = client.SaveAddress(ctx.Context, addrInfo)
		if err != nil {
			return err
		}

		return nil
	},
}
