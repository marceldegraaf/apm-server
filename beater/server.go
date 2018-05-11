package beater

import (
	"context"
	"net"
	"net/http"

	"golang.org/x/net/netutil"

	"github.com/elastic/apm-agent-go"
	"github.com/elastic/apm-agent-go/module/apmhttp"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/version"
)

type reporter func(context.Context, pendingReq) error

func newServer(config *Config, tracer *elasticapm.Tracer, report reporter) *http.Server {
	mux := newMuxer(config, report)

	return &http.Server{
		Addr: config.Host,
		Handler: apmhttp.Wrap(mux,
			apmhttp.WithServerRequestIgnorer(doNotTrace),
			apmhttp.WithTracer(tracer),
		),
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		MaxHeaderBytes: config.MaxHeaderSize,
	}
}

func doNotTrace(req *http.Request) bool {
	if req.RemoteAddr == "pipe" {
		// Don't trace requests coming from self,
		// or we will go into a continuous cycle.
		return true
	}
	if req.URL.Path == HealthCheckURL {
		// Don't trace healthcheck requests.
		return true
	}
	return false
}

func run(server *http.Server, lis net.Listener, config *Config) error {
	logger := logp.NewLogger("server")
	logger.Infof("Starting apm-server [%s built %s]. Hit CTRL-C to stop it.", version.Commit(), version.BuildTime())
	logger.Infof("Listening on: %s", server.Addr)
	switch config.Frontend.isEnabled() {
	case true:
		logger.Info("Frontend endpoints enabled!")
	case false:
		logger.Info("Frontend endpoints disabled")
	}

	if config.MaxConnections > 0 {
		lis = netutil.LimitListener(lis, config.MaxConnections)
		logger.Infof("connections limit set to: %d", config.MaxConnections)
	}

	ssl := config.SSL
	if ssl.isEnabled() {
		return server.ServeTLS(lis, ssl.Cert, ssl.PrivateKey)
	}
	if config.SecretToken != "" {
		logger.Warn("Secret token is set, but SSL is not enabled.")
	}
	return server.Serve(lis)
}

func stop(server *http.Server) {
	logger := logp.NewLogger("server")
	err := server.Shutdown(context.Background())
	if err != nil {
		logger.Error(err.Error())
		err = server.Close()
		if err != nil {
			logger.Error(err.Error())
		}
	}
}
