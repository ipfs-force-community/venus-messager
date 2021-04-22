package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-messager/api"
	"github.com/ipfs-force-community/venus-messager/api/controller"
	"github.com/ipfs-force-community/venus-messager/api/jwt"
	ccli "github.com/ipfs-force-community/venus-messager/cli"
	"github.com/ipfs-force-community/venus-messager/config"
	"github.com/ipfs-force-community/venus-messager/log"
	"github.com/ipfs-force-community/venus-messager/models"
	"github.com/ipfs-force-community/venus-messager/service"
	"github.com/ipfs-force-community/venus-messager/version"
)

func main() {
	app := &cli.App{
		Name:  "venus message",
		Usage: "used for manage message",
		Commands: []*cli.Command{ccli.MsgCmds,
			ccli.AddrCmds,
			ccli.WalletCmds,
			ccli.SharedParamsCmds,
			ccli.NodeCmds,
			ccli.WalletAddrCmds,
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
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "./messager.toml",
			Usage:   "specify config file",
		},

		&cli.StringFlag{
			Name:  "auth-url",
			Usage: "url for auth server",
			Value: "http://127.0.0.1:8989",
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
			Value: "sqlite",
		},
		&cli.StringFlag{
			Name:  "sqlite-path",
			Usage: "sqlite db path",
			Value: "./message.db",
		},
		&cli.StringFlag{
			Name:  "mysql-dsn",
			Usage: "mysql connection string",
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

	lst, err := net.Listen("tcp", cfg.API.Address)
	if err != nil {
		return err
	}

	shutdownChan := make(chan struct{})
	provider := fx.Options(
		fx.Logger(fxLogger{log}),
		//prover
		fx.Supply(cfg, &cfg.DB, &cfg.API, &cfg.JWT, &cfg.Node, &cfg.Log, &cfg.MessageService, &cfg.MessageState, &cfg.Wallet),
		fx.Supply(log),
		fx.Supply(client),
		fx.Supply((ShutdownChan)(shutdownChan)),

		fx.Provide(service.NewMessageState),
		//db
		fx.Provide(models.SetDataBase),
		//service
		service.MessagerService(),
		//api
		fx.Provide(api.InitRouter),
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
		fx.Invoke(controller.SetupController),
		fx.Invoke(service.StartNodeEvents),
		fx.Invoke(api.RunAPI),
	)
	app := fx.New(provider, invoker)
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

	if ctx.IsSet("node-token") {
		cfg.Node.Token = ctx.String("node-token")
	}

	if ctx.IsSet("db-type") {
		cfg.DB.Type = ctx.String("db-type")
		switch cfg.DB.Type {
		case "sqlite":
			if ctx.IsSet("sqlite-path") {
				cfg.DB.Sqlite.Path = ctx.String("sqlite-path")
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
	log *logrus.Logger
}

func (l fxLogger) Printf(str string, args ...interface{}) {
	l.log.Infof(str, args...)
}
