package jwt

import (
	"github.com/filecoin-project/venus-auth/cmd/jwtclient"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/venus-messager/config"
	"github.com/filecoin-project/venus-messager/log"
)

type JwtClient struct {
	Local, Remote jwtclient.IJwtAuthClient
}

func NewJwtClient(log *log.Logger, jwtCfg *config.JWTConfig) (*JwtClient, error) {
	var err error
	jc := &JwtClient{
		Remote: newRemoteJwtClient(jwtCfg),
	}
	log.Infof("auth url: %s", jwtCfg.AuthURL)
	if jc.Local, err = newLocalJWTClient(jwtCfg); err != nil {
		return nil, xerrors.Errorf("new local jwt client failed %v", err)
	}

	return jc, nil
}
