package main

import (
	"fmt"
	"net"
	_ "net/http/pprof"
	"os"
	"path/filepath"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/venus-messager/api"
	"github.com/filecoin-project/venus-messager/api/jwt"
	ccli "github.com/filecoin-project/venus-messager/cli"
	"github.com/filecoin-project/venus-messager/config"
	"github.com/filecoin-project/venus-messager/gateway"
	"github.com/filecoin-project/venus-messager/log"
	"github.com/filecoin-project/venus-messager/models"
	"github.com/filecoin-project/venus-messager/service"
	"github.com/filecoin-project/venus-messager/version"
)

func main() {
	app := &cli.App{
		Name:  "venus message",
		Usage: "used for manage message",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "./messager.toml",
				Usage:   "specify config file",
			},
		},
		Commands: []*cli.Command{ccli.MsgCmds,
			ccli.AddrCmds,
			ccli.SharedParamsCmds,
			ccli.NodeCmds,
			ccli.LogCmds,
			ccli.SendCmd,
			runCmd,
		},
	}
	app.Version = version.Version + "--" + version.GitCommit
	app.Setup()
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		return
	}

}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "run messager",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "auth-url",
			Usage: "url for auth server",
		},
		&cli.StringFlag{
			Name:  "auth-token",
			Usage: "auth token",
		},

		//node
		&cli.StringFlag{
			Name:  "node-url",
			Usage: "url for connection lotus/venus",
		},
		&cli.StringFlag{
			Name:  "node-token",
			Usage: "token auth for lotus/venus",
		},

		//database
		&cli.StringFlag{
			Name:  "db-type",
			Usage: "which db to use. sqlite/mysql",
		},
		&cli.StringFlag{
			Name:  "sqlite-file",
			Usage: "the path and file name of SQLite, eg. ~/sqlite/message.db",
		},
		&cli.StringFlag{
			Name:  "mysql-dsn",
			Usage: "mysql connection string",
		},

		&cli.StringFlag{
			Name:  "gateway-url",
			Usage: "gateway url",
		},
		&cli.StringFlag{
			Name:  "gateway-token",
			Usage: "gateway token",
		},
	},
	Action: runAction,
}

func runAction(ctx *cli.Context) error {
	path := ctx.String("config")
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	exit, err := config.ConfigExit(path)
	if err != nil {
		return err
	}

	var cfg *config.Config
	if !exit {
		cfg = config.DefaultConfig()
		err = updateFlag(cfg, ctx)
		if err != nil {
			return err
		}
		err = config.WriteConfig(path, cfg)
		if err != nil {
			return err
		}
	} else {
		cfg, err = config.ReadConfig(path)
		if err != nil {
			return err
		}
		err = updateFlag(cfg, ctx)
		if err != nil {
			return err
		}
	}

	if err := config.CheckFile(cfg); err != nil {
		return err
	}

	log, err := log.SetLogger(&cfg.Log)
	if err != nil {
		return err
	}

	client, closer, err := service.NewNodeClient(ctx.Context, &cfg.Node)
	if err != nil {
		return err
	}
	defer closer()

	mAddr, err := ma.NewMultiaddr(cfg.API.Address)
	if err != nil {
		return err
	}

	var walletClient *gateway.IWalletCli
	gatewayProvider := fx.Options()
	if !cfg.Gateway.RemoteEnable { // use local gateway
		gatewayService := gateway.NewGatewayService(&cfg.Gateway)
		walletClient = &gateway.IWalletCli{IWalletClient: gatewayService}
		gatewayProvider = fx.Options(fx.Supply(gatewayService))
	} else {
		walletCli, walletCliCloser, err := gateway.NewWalletClient(&cfg.Gateway)
		walletClient = &gateway.IWalletCli{IWalletClient: walletCli}
		if err != nil {
			return err
		}
		defer walletCliCloser()
	}

	// Listen on the configured address in order to bind the port number in case it has
	// been configured as zero (i.e. OS-provided)
	apiListener, err := manet.Listen(mAddr)
	if err != nil {
		return err
	}
	lst := manet.NetListener(apiListener)

	shutdownChan := make(chan struct{})
	provider := fx.Options(
		fx.Logger(fxLogger{log}),
		//prover
		fx.Supply(cfg, &cfg.DB, &cfg.API, &cfg.JWT, &cfg.Node, &cfg.Log, &cfg.MessageService, &cfg.MessageState, &cfg.Wallet, &cfg.Gateway),
		fx.Supply(log),
		fx.Supply(client),
		fx.Supply(walletClient),
		fx.Supply((ShutdownChan)(shutdownChan)),

		fx.Provide(service.NewMessageState),
		//db
		fx.Provide(models.SetDataBase),
		//service
		service.MessagerService(),
		//api
		fx.Provide(api.NewMessageImp),
		//jwt
		fx.Provide(jwt.NewJwtClient),
		//middleware

		fx.Provide(func() net.Listener {
			return lst
		}),
	)

	invoker := fx.Options(
		//invoke
		fx.Invoke(models.AutoMigrate),
		fx.Invoke(service.StartNodeEvents),
		fx.Invoke(api.RunAPI),
	)
	app := fx.New(gatewayProvider, provider, invoker)
	if err := app.Start(ctx.Context); err != nil {
		// comment fx.NopLogger few lines above for easier debugging
		return xerrors.Errorf("starting node: %w", err)
	}

	go func() {
		<-shutdownChan
		log.Warn("received shutdown")

		log.Warn("Shutting down...")
		if err := app.Stop(ctx.Context); err != nil {
			log.Errorf("graceful shutting down failed: %s", err)
		}
		log.Warn("Graceful shutdown successful")
	}()

	<-app.Done()
	return nil
}

func updateFlag(cfg *config.Config, ctx *cli.Context) error {
	if ctx.IsSet("auth-url") {
		cfg.JWT.Url = ctx.String("auth-url")
	}

	if ctx.IsSet("node-url") {
		cfg.Node.Url = ctx.String("node-url")
	}

	if ctx.IsSet("gateway-url") {
		cfg.Gateway.RemoteEnable = true
		cfg.Gateway.Url = ctx.String("gateway-url")
	}

	if ctx.IsSet("auth-token") {
		cfg.Node.Token = ctx.String("auth-token")
		cfg.Gateway.Token = ctx.String("auth-token")
	}

	if ctx.IsSet("node-token") {
		cfg.Node.Token = ctx.String("node-token")
	}

	if ctx.IsSet("gateway-token") {
		cfg.Gateway.Token = ctx.String("gateway-token")
	}

	if ctx.IsSet("db-type") {
		cfg.DB.Type = ctx.String("db-type")
		switch cfg.DB.Type {
		case "sqlite":
			if ctx.IsSet("sqlite-file") {
				cfg.DB.Sqlite.File = ctx.String("sqlite-file")
			}
		case "mysql":
			if ctx.IsSet("mysql-dsn") {
				cfg.DB.MySql.ConnectionString = ctx.String("mysql-dsn")
			}
		default:
			return xerrors.New("unsupport db type")
		}
	}
	return nil
}

type fxLogger struct {
	log *log.Logger
}

func (l fxLogger) Printf(str string, args ...interface{}) {
	l.log.Infof(str, args...)
}
