package natsrv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/config"
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
	if config.NATSModeExternal == cfg.Mode {
		return startExternal(ctx, cfg, internalSecret, logger)
	}

	identity, err := resolveClientIdentity(cfg.Auth, internalSecret)
	if nil != err {
		return nil, fmt.Errorf("nats: resolve client credentials: %w", err)
	}
	opts, err := BuildOptions(cfg, peers)
	if nil != err {
		return nil, fmt.Errorf("nats: build options: %w", err)
	}
	if err := applyServerAuth(opts, cfg, internalSecret, identity); nil != err {
		return nil, fmt.Errorf("nats: configure auth: %w", err)
	}
	inProcessTLS, err := inProcessTLSConfig(cfg.TLS)
	if nil != err {
		return nil, fmt.Errorf("nats: in-process tls: %w", err)
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

	clientOpts := []nats.Option{
		nats.InProcessServer(ns),
		nats.Name("callfs-" + cfg.ServerName),
		nats.MaxReconnects(-1),
	}
	clientOpts = append(clientOpts, identity.options...)
	clientOpts = append(clientOpts, lifecycleOptions(logger)...)
	if nil != inProcessTLS {
		clientOpts = append(clientOpts, nats.Secure(inProcessTLS))
	}
	nc, err := nats.Connect("", clientOpts...)
	if nil != err {
		stopEmbedded(ns, nil)
		return nil, fmt.Errorf("nats: connect in-process: %w", err)
	}

	js, err := jetstream.New(nc)
	if nil != err {
		stopEmbedded(ns, nc)
		return nil, fmt.Errorf("nats: create jetstream: %w", err)
	}

	if err := waitJetStream(ctx, js); nil != err {
		stopEmbedded(ns, nc)
		return nil, err
	}

	logger.Info("NATS server started", zap.String("server_name", cfg.ServerName), zap.String("mode", cfg.Mode))
	return &Server{ns: ns, nc: nc, js: js, logger: logger}, nil
}

func startExternal(ctx context.Context, cfg config.NATSConfig, internalSecret string, logger *zap.Logger) (*Server, error) {
	identity, err := resolveClientIdentity(cfg.Auth, internalSecret)
	if nil != err {
		return nil, fmt.Errorf("nats: resolve client credentials: %w", err)
	}

	clientOpts := []nats.Option{
		nats.Name("callfs-" + cfg.ServerName),
		nats.MaxReconnects(-1),
	}
	clientOpts = append(clientOpts, identity.options...)
	clientOpts = append(clientOpts, lifecycleOptions(logger)...)
	if "" != cfg.External.TLSCAFile {
		clientOpts = append(clientOpts, nats.RootCAs(cfg.External.TLSCAFile))
	}
	if "" != cfg.TLS.ClientCertFile {
		clientOpts = append(clientOpts, nats.ClientCert(cfg.TLS.ClientCertFile, cfg.TLS.ClientKeyFile))
	}
	nc, err := nats.Connect(strings.Join(cfg.External.URLs, ","), clientOpts...)
	if nil != err {
		return nil, fmt.Errorf("nats: connect to external servers: %w", err)
	}

	var js jetstream.JetStream
	if "" == cfg.External.JSDomain {
		js, err = jetstream.New(nc)
	} else {
		js, err = jetstream.NewWithDomain(nc, cfg.External.JSDomain)
	}
	if nil != err {
		nc.Close()
		return nil, fmt.Errorf("nats: create jetstream: %w", err)
	}

	if err := waitJetStream(ctx, js); nil != err {
		nc.Close()
		return nil, err
	}

	logger.Info("NATS client connected to external servers", zap.String("server_name", cfg.ServerName))
	return &Server{nc: nc, js: js, logger: logger}, nil
}

func waitJetStream(ctx context.Context, js jetstream.JetStream) error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := js.AccountInfo(attemptCtx)
		attemptCancel()
		if nil == err {
			return nil
		}
		if nil != ctx.Err() || !time.Now().Before(deadline) {
			return fmt.Errorf("nats: jetstream not ready: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
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

func lifecycleOptions(logger *zap.Logger) []nats.Option {
	return []nats.Option{
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("NATS connection lost", zap.Error(err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS connection restored", zap.String("url", nc.ConnectedUrl()))
		}),
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("NATS server shutting down")
	err := s.shutdown(ctx)
	if nil != err {
		s.logger.Warn("NATS server shutdown failed", zap.Error(err))
		return err
	}
	s.logger.Info("NATS server stopped")
	return nil
}

func (s *Server) shutdown(ctx context.Context) error {
	if nil == s.ns {
		return drainConn(ctx, s.nc)
	}
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

func drainConn(ctx context.Context, nc *nats.Conn) error {
	if nil == nc {
		return nil
	}
	if err := nc.Drain(); nil != err && !errors.Is(err, nats.ErrConnectionClosed) {
		nc.Close()
		return fmt.Errorf("nats: drain connection: %w", err)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for !nc.IsClosed() {
		select {
		case <-ctx.Done():
			nc.Close()
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return nil
}
