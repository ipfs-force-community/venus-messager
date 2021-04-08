package api

import (
	"context"
	"encoding/json"
	"golang.org/x/xerrors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	ginlogrus "github.com/toorop/gin-logrus"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-messager/api/controller"
	"github.com/ipfs-force-community/venus-messager/api/jwt"
	"github.com/ipfs-force-community/venus-messager/types"
)

type JsonRpcRequest struct {
	// common
	Jsonrpc string            `json:"jsonrpc"`
	ID      int64             `json:"id,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`

	// request
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
}

type RewriteJsonRpcToRestful struct {
	*gin.Engine
}

func (r *RewriteJsonRpcToRestful) PreRequest(w http.ResponseWriter, req *http.Request) (int, error) {
	if req.Method == http.MethodPost && req.URL.Path == "/rpc/v0" {
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return 503, xerrors.New("failed to read json rpc body")
		}

		jsonReq := &JsonRpcRequest{}
		err = json.Unmarshal(body, jsonReq)
		if err != nil {
			return 503, xerrors.New("failed to unmarshal json rpc body")
		}
		methodSeq := strings.Split(jsonReq.Method, ".")
		//	methodPath := strings.Join(strings.Split(jsonReq.Method, "."), "/")
		newRequestUrl := req.RequestURI + "/" + methodSeq[len(methodSeq)-1] + "/" + strconv.FormatInt(jsonReq.ID, 10)
		newUrl, err := url.Parse(newRequestUrl)
		if err != nil {
			return 503, xerrors.New("failed to parser new url")
		}
		req.URL = newUrl
		req.RequestURI = newRequestUrl
		params, _ := json.Marshal(jsonReq.Params)

		ctx := context.WithValue(req.Context(), types.Arguments{}, map[string]interface{}{
			"method": methodSeq[len(methodSeq)-1],
			"params": params,
			"id":     jsonReq.ID,
		})
		newReq := req.WithContext(ctx)
		*req = *newReq
	}
	return 0, nil
}

func InitRouter(log *logrus.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode) //todo change mode in cmds
	g := gin.New()
	g.Use(ginlogrus.Logger(log), gin.Recovery())
	return g
}

func RunAPI(lc fx.Lifecycle, r *gin.Engine, jwtClient jwt.IJwtClient, lst net.Listener, log *logrus.Logger) error {
	rewriteJsonRpc := &RewriteJsonRpcToRestful{
		Engine: r,
	}
	filter := controller.NewJWTFilter(jwtClient, log, r)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		code, err := rewriteJsonRpc.PreRequest(writer, request)
		if err != nil {
			writer.WriteHeader(code)
			log.Errorf("cannot transfser jsonrpc to rustful")
			return
		}

		code, err = filter.PreRequest(writer, request)
		if err != nil {
			resp := controller.JsonRpcResponse{
				ID: request.Context().Value(types.Arguments{}).(map[string]interface{})["id"].(int64),
				Error: &controller.RespError{
					Code:    code,
					Message: err.Error(),
				},
			}
			writer.WriteHeader(code)
			data, _ := json.Marshal(resp)
			_, _ = writer.Write(data)
			log.Errorf("cannot auth token verify")
			return
		}

		r.ServeHTTP(writer, request)
	})

	apiserv := &http.Server{
		Handler: handler,
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				log.Info("Start rpcserver ", lst.Addr())
				if err := apiserv.Serve(lst); err != nil {
					log.Errorf("Start rpcserver failed: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return lst.Close()
		},
	})
	return nil
}
