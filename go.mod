module github.com/filecoin-project/venus-messager

go 1.15

require (
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d
	github.com/fatih/color v1.10.0
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/venus v1.0.1-0.20210707073618-62e8cf9a7834
	github.com/filecoin-project/venus-auth v1.1.1-0.20210601064545-55f3162444fd
	github.com/filecoin-project/venus-wallet v1.1.0
	github.com/gin-gonic/gin v1.6.3
	github.com/google/uuid v1.2.0
	github.com/hraban/lrucache v0.0.0-20201130153820-17052bf09781 // indirect
	github.com/hunjixin/automapper v0.0.0-20191127090318-9b979ce72ce2
	github.com/ipfs-force-community/venus-gateway v0.0.0-20210528060921-460ec6185a7d
	github.com/ipfs/go-cid v0.0.7
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pelletier/go-toml v1.6.0
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.3.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20210219115102-f37d292932f2
	go.uber.org/fx v1.13.1
	golang.org/x/lint v0.0.0-20201208152925-83fdc39ff7b5 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gorm.io/driver/mysql v1.0.5
	gorm.io/driver/sqlite v1.1.4
	gorm.io/gorm v1.21.3
	modernc.org/mathutil v1.1.1
)

replace github.com/filecoin-project/go-jsonrpc => github.com/ipfs-force-community/go-jsonrpc v0.1.4-0.20210708021325-1ca28be4e5a3
