package natsrv

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ebogdum/callfs/config"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type Server struct {
	ns     *server.Server
	nc     *nats.Conn
	js     jetstream.JetStream
	logger *zap.Logger
}

func Start(ctx context.Context, cfg config.NATSConfig, internalSecret string, peers map[string]string, logger *zap.Logger) (*Server, error) {
	creds, err := DeriveCredentials(internalSecret)
	if nil != err {
		return nil, fmt.Errorf("nats: derive credentials: %w", err)
	}
	opts, err := BuildOptions(cfg, peers)
	if nil != err {
		return nil, fmt.Errorf("nats: build options: %w", err)
	}
	opts.Nkeys = []*server.NkeyUser{{Nkey: creds.UserPublic}}
	opts.Cluster.Username = creds.RouteUser
	opts.Cluster.Password = creds.RoutePassword
	for _, route := range opts.Routes {
		route.User = url.UserPassword(creds.RouteUser, creds.RoutePassword)
	}

	ns, err := server.NewServer(opts)
	if nil != err {
		return nil, fmt.Errorf("nats: create server: %w", err)
	}
	ns.SetLoggerV2(zapLogger{logger: logger.Named("nats")}, false, false, false)
	ns.Start()
	if !ns.ReadyForConnections(30 * time.Second) {
		stopEmbedded(ns, nil)
		return nil, errors.New("nats: server did not become ready")
	}

	nc, err := nats.Connect("",
		nats.InProcessServer(ns),
		nats.Nkey(creds.UserPublic, creds.User.Sign),
		nats.Name("callfs-"+cfg.ServerName),
		nats.MaxReconnects(-1),
	)
	if nil != err {
		stopEmbedded(ns, nil)
		return nil, fmt.Errorf("nats: connect in-process: %w", err)
	}

	js, err := jetstream.New(nc)
	if nil != err {
		stopEmbedded(ns, nc)
		return nil, fmt.Errorf("nats: create jetstream: %w", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 2*time.Second)
		_, err = js.AccountInfo(attemptCtx)
		attemptCancel()
		if nil == err {
			break
		}
		if nil != ctx.Err() || !time.Now().Before(deadline) {
			stopEmbedded(ns, nc)
			return nil, fmt.Errorf("nats: jetstream not ready: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	return &Server{ns: ns, nc: nc, js: js, logger: logger}, nil
}

func stopEmbedded(ns *server.Server, nc *nats.Conn) {
	if nil != nc {
		nc.Close()
	}
	ns.Shutdown()
	ns.WaitForShutdown()
}

func (s *Server) Conn() *nats.Conn {
	return s.nc
}

func (s *Server) JetStream() jetstream.JetStream {
	return s.js
}

func (s *Server) Embedded() *server.Server {
	return s.ns
}

func (s *Server) Shutdown(ctx context.Context) error {
	if nil != s.nc {
		_ = s.nc.Drain()
	}
	s.ns.Shutdown()
	done := make(chan struct{})
	go func() {
		s.ns.WaitForShutdown()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
