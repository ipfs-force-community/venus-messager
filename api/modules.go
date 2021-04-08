package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

var _ io.ReadCloser = (*CloserReader)(nil)

type CloserReader struct {
	reader io.Reader
}

func (c *CloserReader) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

func (c *CloserReader) Close() error {
	return nil
}

type RewriteJsonRpcToRestful struct {
	*gin.Engine
}

func (r *RewriteJsonRpcToRestful) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost && req.URL.Path == "/rpc/v0" {
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("failed to read json rpc body"))
			return
		}

		jsonReq := &JsonRpcRequest{}
		err = json.Unmarshal(body, jsonReq)
		if err != nil {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("failed to unmarshal json rpc body"))
			return
		}
		methodSeq := strings.Split(jsonReq.Method, ".")
		//	methodPath := strings.Join(strings.Split(jsonReq.Method, "."), "/")
		newRequestUrl := req.RequestURI + "/" + methodSeq[len(methodSeq)-1] + "/" + strconv.FormatInt(jsonReq.ID, 10)
		newUrl, err := url.Parse(newRequestUrl)
		if err != nil {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("failed to parser new url"))
			return
		}
		req.URL = newUrl
		req.RequestURI = newRequestUrl
		params, _ := json.Marshal(jsonReq.Params)
		req.Body = &CloserReader{bytes.NewBuffer(params)}
	}

	r.Engine.ServeHTTP(w, req)
}

func UseMiddleware(log *logrus.Logger, r *gin.Engine) error {

	//r.Use(middleware.RewriteJsonRpcMiddleware)
	return nil
}

func InitRouter(log *logrus.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Use(ginlogrus.Logger(log), gin.Recovery())
	return g
}

func RunAPI(lc fx.Lifecycle, r *gin.Engine, lst net.Listener, log *logrus.Logger) error {
	skipContextPathRouter := &RewriteJsonRpcToRestful{
		Engine: r,
	}

	handler := http.NewServeMux()
	handler.Handle("/", skipContextPathRouter)
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
