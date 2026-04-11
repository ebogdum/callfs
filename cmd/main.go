package main

//	@title		CallFS API
//	@version	1.3.0
//	@description	CallFS is an ultra-lightweight, high-performance REST API filesystem that provides precise Linux filesystem semantics over various backends.
//	@termsOfService	http://swagger.io/terms/

//	@contact.name	CallFS Support
//	@contact.url	http://callfs.io/support
//	@contact.email	support@callfs.io

//	@license.name	MIT
//	@license.url	https://opensource.org/licenses/MIT

//	@host		localhost:8443
//	@BasePath	/v1

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/backends/internalproxy"
	"github.com/ebogdum/callfs/backends/localfs"
	"github.com/ebogdum/callfs/backends/noop"
	"github.com/ebogdum/callfs/backends/s3"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/postgres"
	metadataraft "github.com/ebogdum/callfs/metadata/raft"
	metadataredis "github.com/ebogdum/callfs/metadata/redis"
	"github.com/ebogdum/callfs/metadata/schema"
	metadatasqlite "github.com/ebogdum/callfs/metadata/sqlite"
	"github.com/ebogdum/callfs/server"
	"github.com/ebogdum/callfs/server/handlers"
)

var rootCmd = &cobra.Command{
	Use:   "callfs",
	Short: "CallFS - Ultra-lightweight REST API filesystem",
	Long: `CallFS is an ultra-lightweight, high-performance REST API filesystem 
that provides precise Linux filesystem semantics over various backends.`,
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the CallFS server",
	Long:  "Start the CallFS server with the configured backends and API endpoints",
	RunE:  runServer,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cluster management commands",
}

var clusterJoinCmd = &cobra.Command{
	Use:   "join",
	Short: "Join this node to a Raft metadata cluster",
	RunE:  runClusterJoin,
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	Long:  "Validate the CallFS configuration and display the loaded settings",
	RunE:  validateConfig,
}

var configFilePath string
var joinLeaderURL string
var joinNodeID string
var joinRaftAddr string
var joinAPIEndpoint string
var joinInternalSecret string

func main() {
	// Add flags to server command
	serverCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	configCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	clusterCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	clusterJoinCmd.Flags().StringVar(&joinLeaderURL, "leader", "", "Leader API URL (e.g. http://10.0.0.1:8443)")
	clusterJoinCmd.Flags().StringVar(&joinNodeID, "node-id", "", "Joining node ID")
	clusterJoinCmd.Flags().StringVar(&joinRaftAddr, "raft-addr", "", "Joining node Raft address (e.g. 10.0.0.2:7000)")
	clusterJoinCmd.Flags().StringVar(&joinAPIEndpoint, "api-endpoint", "", "Joining node API endpoint (e.g. http://10.0.0.2:8443)")
	clusterJoinCmd.Flags().StringVar(&joinInternalSecret, "internal-secret", "", "Shared internal proxy secret")
	_ = clusterJoinCmd.MarkFlagRequired("leader")
	clusterCmd.AddCommand(clusterJoinCmd)

	// Add subcommands
	configCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(serverCmd, configCmd, clusterCmd)

	// If no command specified, default to server
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "server")
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func populateJoinFlagsFromConfig() {
	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err != nil {
		return
	}
	defaults := []struct {
		flag     *string
		cfgValue string
	}{
		{&joinNodeID, cfg.Raft.NodeID},
		{&joinRaftAddr, cfg.Raft.BindAddr},
		{&joinAPIEndpoint, cfg.Server.ExternalURL},
		{&joinInternalSecret, cfg.Auth.InternalProxySecret},
	}
	for _, d := range defaults {
		if strings.TrimSpace(*d.flag) == "" {
			*d.flag = strings.TrimSpace(d.cfgValue)
		}
	}
}

func runClusterJoin(cmd *cobra.Command, args []string) error {
	populateJoinFlagsFromConfig()

	joinNodeID = strings.TrimSpace(joinNodeID)
	joinRaftAddr = strings.TrimSpace(joinRaftAddr)
	joinAPIEndpoint = strings.TrimSpace(joinAPIEndpoint)
	joinInternalSecret = strings.TrimSpace(joinInternalSecret)

	requiredFlags := []struct {
		value string
		name  string
	}{
		{joinNodeID, "node id (use --node-id or set raft.node_id in config)"},
		{joinRaftAddr, "raft address (use --raft-addr or set raft.bind_addr in config)"},
		{joinAPIEndpoint, "api endpoint (use --api-endpoint or set server.external_url in config)"},
		{joinInternalSecret, "internal secret (use --internal-secret or set auth.internal_proxy_secret in config)"},
	}
	for _, f := range requiredFlags {
		if f.value == "" {
			return fmt.Errorf("%s is required", f.name)
		}
	}

	payload := metadataraft.JoinRequest{
		NodeID:      joinNodeID,
		RaftAddr:    joinRaftAddr,
		APIEndpoint: joinAPIEndpoint,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	url := strings.TrimRight(joinLeaderURL, "/") + "/v1/internal/raft/join"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create join request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", joinInternalSecret))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contact leader: %w", err)
	}
	defer resp.Body.Close()

	var out metadataraft.JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("failed to decode join response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			return fmt.Errorf("join failed: %s", out.Error)
		}
		return fmt.Errorf("join failed with status %d", resp.StatusCode)
	}

	fmt.Printf("Join successful: node=%s leader=%s status=%s\n", joinNodeID, out.LeaderID, out.Status)
	return nil
}

func initMetadataStore(cfg *config.AppConfig, logger *zap.Logger) (metadata.Store, *metadataraft.Store, error) {
	metadataStoreType := strings.ToLower(strings.TrimSpace(cfg.MetadataStore.Type))
	switch metadataStoreType {
	case "raft":
		apiPeers := make(map[string]string, len(cfg.Raft.APIPeerEndpoints)+1)
		maps.Copy(apiPeers, cfg.Raft.APIPeerEndpoints)
		if _, exists := apiPeers[cfg.Raft.NodeID]; !exists {
			apiPeers[cfg.Raft.NodeID] = cfg.Server.ExternalURL
		}

		store, err := metadataraft.NewRaftStore(metadataraft.Config{
			NodeID:              cfg.Raft.NodeID,
			BindAddr:            cfg.Raft.BindAddr,
			DataDir:             cfg.Raft.DataDir,
			Bootstrap:           cfg.Raft.Bootstrap,
			Peers:               cfg.Raft.Peers,
			APIPeerEndpoints:    apiPeers,
			ApplyTimeout:        cfg.Raft.ApplyTimeout,
			ForwardTimeout:      cfg.Raft.ForwardTimeout,
			SnapshotInterval:    cfg.Raft.SnapshotInterval,
			SnapshotThreshold:   cfg.Raft.SnapshotThreshold,
			RetainSnapshotCount: cfg.Raft.RetainSnapshotCount,
			InternalAuthToken:   cfg.Auth.InternalProxySecret,
		}, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize raft metadata store: %w", err)
		}
		return store, store, nil
	case "sqlite":
		store, err := metadatasqlite.NewSQLiteStore(cfg.MetadataStore.SQLitePath, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize sqlite metadata store: %w", err)
		}
		return store, nil, nil
	case "redis":
		store, err := metadataredis.NewRedisStore(
			cfg.MetadataStore.RedisAddr,
			cfg.MetadataStore.RedisPassword,
			cfg.MetadataStore.RedisDB,
			cfg.MetadataStore.RedisKeyPrefix,
			logger,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize redis metadata store: %w", err)
		}
		return store, nil, nil
	case "postgres":
		logger.Info("Running database migrations")
		if err := schema.RunMigrations(cfg.MetadataStore.DSN); err != nil {
			return nil, nil, fmt.Errorf("failed to run database migrations: %w", err)
		}
		store, err := postgres.NewPostgresStore(cfg.MetadataStore.DSN, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize postgres metadata store: %w", err)
		}
		return store, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported metadata store type: %s", cfg.MetadataStore.Type)
	}
}

func initLockManager(cfg *config.AppConfig, logger *zap.Logger) (locks.Manager, error) {
	dlmType := strings.ToLower(strings.TrimSpace(cfg.DLM.Type))
	switch dlmType {
	case "local":
		return locks.NewLocalManager(), nil
	case "redis":
		manager, err := locks.NewRedisManager(cfg.DLM.RedisAddr, cfg.DLM.RedisPassword, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize redis lock manager: %w", err)
		}
		return manager, nil
	default:
		return nil, fmt.Errorf("unsupported dlm type: %s", cfg.DLM.Type)
	}
}

func initBackends(cfg *config.AppConfig, logger *zap.Logger) (localFS, s3Back, proxyBack backends.Storage, proxyAdapter *internalproxy.InternalProxyAdapter, err error) {
	if cfg.Backend.LocalFSRootPath != "" {
		logger.Info("Initializing LocalFS backend", zap.String("root_path", cfg.Backend.LocalFSRootPath))
		localFS, err = localfs.NewLocalFSAdapter(cfg.Backend.LocalFSRootPath)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to initialize LocalFS backend: %w", err)
		}
	} else {
		logger.Info("LocalFS backend disabled (no root path configured)")
		localFS = noop.NewNoopAdapter()
	}

	if cfg.Backend.S3BucketName != "" {
		logger.Info("Initializing S3 backend", zap.String("bucket", cfg.Backend.S3BucketName))
		s3Back, err = s3.NewS3Adapter(cfg.Backend, logger)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to initialize S3 backend: %w", err)
		}
	} else {
		logger.Info("S3 backend disabled (no bucket configured)")
		s3Back = noop.NewNoopAdapter()
	}

	if len(cfg.InstanceDiscovery.PeerEndpoints) > 0 {
		logger.Info("Initializing internal proxy backend", zap.Int("peer_count", len(cfg.InstanceDiscovery.PeerEndpoints)))
		adapter, adapterErr := internalproxy.NewInternalProxyAdapter(
			cfg.InstanceDiscovery.PeerEndpoints,
			cfg.Auth.InternalProxySecret,
			cfg.Backend.InternalProxySkipTLSVerify,
			logger)
		if adapterErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to initialize internal proxy backend: %w", adapterErr)
		}
		proxyAdapter = adapter
		proxyBack = adapter
	} else {
		logger.Info("Internal proxy backend disabled (no peers configured)")
		proxyBack = noop.NewNoopAdapter()
	}

	return localFS, s3Back, proxyBack, proxyAdapter, nil
}

func initErasureManager(cfg *config.AppConfig, metadataStore metadata.Store, localFSBackend, s3Backend backends.Storage, logger *zap.Logger) (*erasure.Manager, error) {
	erasureMetaStore, ok := metadataStore.(metadata.ErasureMetadataStore)
	if !ok {
		return nil, fmt.Errorf("metadata store type %s does not support erasure coding", cfg.MetadataStore.Type)
	}

	var shardBackend backends.Storage
	shardBackendType := strings.ToLower(strings.TrimSpace(cfg.Erasure.ShardBackend))
	if shardBackendType == "s3" {
		shardBackend = s3Backend
	} else {
		shardBackend = localFSBackend
	}

	erasurePeers := make(map[string]string)
	maps.Copy(erasurePeers, cfg.InstanceDiscovery.PeerEndpoints)
	if cfg.Server.ExternalURL != "" {
		erasurePeers[cfg.InstanceDiscovery.InstanceID] = cfg.Server.ExternalURL
	}

	return erasure.NewManager(
		erasureMetaStore,
		shardBackend,
		&cfg.Erasure,
		cfg.InstanceDiscovery.InstanceID,
		erasurePeers,
		cfg.Auth.InternalProxySecret,
		logger,
	), nil
}

func ensureRaftRootDirectory(cfg *config.AppConfig, raftStore *metadataraft.Store, coreEngine *core.Engine, logger *zap.Logger) {
	if cfg.Raft.Bootstrap {
		waitDeadline := time.Now().Add(8 * time.Second)
		for !raftStore.IsLeader() && time.Now().Before(waitDeadline) {
			time.Sleep(200 * time.Millisecond)
		}
	}

	if raftStore.IsLeader() {
		if err := coreEngine.EnsureRootDirectory(context.Background()); err != nil {
			logger.Fatal("Failed to ensure root directory exists", zap.Error(err))
		}
		if snapErr := raftStore.TakeSnapshot(); snapErr != nil {
			logger.Warn("Initial snapshot after root directory creation failed", zap.Error(snapErr))
		} else {
			logger.Info("Initial snapshot taken for follower catch-up")
		}
	} else {
		logger.Info("Skipping root directory bootstrap on follower node",
			zap.String("node_id", cfg.Raft.NodeID),
			zap.String("leader_id", raftStore.LeaderID()))
	}
}

func buildHTTPHandler(ctx context.Context, cfg *config.AppConfig, coreEngine *core.Engine, metadataStore metadata.Store, localFSBackend backends.Storage, raftMetadataStore *metadataraft.Store, logger *zap.Logger) (http.Handler, *server.RouterResources, error) {
	logger.Info("Initializing authentication and authorization")
	authenticator := auth.NewAPIKeyAuthenticator(cfg.Auth.APIKeys, cfg.Auth.InternalProxySecret)
	authorizer := auth.NewUnixAuthorizer(metadataStore)

	logger.Info("Initializing link manager")
	linkManager, err := links.NewLinkManager(metadataStore, cfg.Auth.SingleUseLinkSecret, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize link manager: %w", err)
	}

	links.StartCleanupWorker(ctx, metadataStore, 5*time.Minute, logger)

	logger.Info("Initializing HTTP router")
	router, routerResources := server.NewRouter(coreEngine, authenticator, authorizer, linkManager, &cfg.Server, &cfg.Backend, cfg.Server.ExternalURL, logger)
	rootHandler := http.Handler(router)

	if cfg.Erasure.Enabled {
		rootHandler = registerErasureRoutes(rootHandler, localFSBackend, cfg.Auth.InternalProxySecret, logger)
	}

	if raftMetadataStore != nil {
		rootHandler = registerRaftRoutes(rootHandler, cfg, raftMetadataStore, coreEngine, logger)
	}

	return rootHandler, routerResources, nil
}

func registerErasureRoutes(handler http.Handler, localFSBackend backends.Storage, internalSecret string, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("/v1/internal/shards/", recoverMiddleware(logger, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			handlers.InternalStoreShardHandler(localFSBackend, internalSecret, logger)(w, r)
		case http.MethodGet:
			handlers.InternalGetShardHandler(localFSBackend, internalSecret, logger)(w, r)
		case http.MethodDelete:
			handlers.InternalDeleteShardHandler(localFSBackend, internalSecret, logger)(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return mux
}

func registerRaftRoutes(handler http.Handler, cfg *config.AppConfig, raftStore *metadataraft.Store, coreEngine *core.Engine, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("/v1/internal/raft/join", recoverMiddleware(logger, handleRaftJoin(cfg, raftStore, coreEngine, logger)))
	mux.HandleFunc("/v1/internal/raft/metadata/apply", recoverMiddleware(logger, handleRaftApply(cfg, raftStore, logger)))
	return mux
}

func handleRaftJoin(cfg *config.AppConfig, raftStore *metadataraft.Store, coreEngine *core.Engine, _ *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		authHeader := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(cfg.Auth.InternalProxySecret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: "unauthorized"})
			return
		}

		if !raftStore.IsLeader() {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: "not leader", LeaderID: raftStore.LeaderID()})
			return
		}

		var req metadataraft.JoinRequest
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: fmt.Sprintf("invalid request: %v", err)})
			return
		}

		if err := raftStore.AddVoter(r.Context(), req.NodeID, req.RaftAddr, req.APIEndpoint); err != nil {
			status := http.StatusBadGateway
			if strings.Contains(strings.ToLower(err.Error()), "required") {
				status = http.StatusBadRequest
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: err.Error(), LeaderID: raftStore.LeaderID()})
			return
		}

		coreEngine.SetPeerEndpoint(req.NodeID, req.APIEndpoint)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "joined", LeaderID: raftStore.LeaderID()})
	}
}

func handleRaftApply(cfg *config.AppConfig, raftStore *metadataraft.Store, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		authHeader := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(cfg.Auth.InternalProxySecret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: "unauthorized"})
			return
		}

		var req metadataraft.ForwardApplyRequest
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			return
		}

		res, err := raftStore.ApplyForwardedCommand(r.Context(), req.Command)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			errCode := err.Error()
			if errors.Is(err, metadata.ErrNotFound) {
				errCode = "not_found"
			}
			if errors.Is(err, metadata.ErrAlreadyExists) {
				errCode = "already_exists"
			}
			_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: errCode})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := metadataraft.ForwardApplyResponse{CleanupCount: res.CleanupCount}
		if res.Metadata != nil {
			resp.Metadata = res.Metadata
		}
		if res.Link != nil {
			resp.Link = res.Link
		}
		if res.Children != nil {
			resp.Children = res.Children
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("Failed to encode raft apply response", zap.Error(err))
		}
	}
}

func startServer(srv *http.Server, cfg *config.AppConfig, logger *zap.Logger) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		protocol := strings.ToLower(cfg.Server.Protocol)
		if protocol == "" {
			protocol = "https"
		}

		switch protocol {
		case "http":
			logger.Info("Starting HTTP server", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTP server failed: %w", err)
			}
		case "auto":
			if cfg.Server.CertFile != "" && cfg.Server.KeyFile != "" {
				logger.Info("Starting HTTPS server (auto mode)", zap.String("addr", cfg.Server.ListenAddr))
				if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("HTTPS server (auto) failed: %w", err)
				}
				return
			}
			logger.Info("Starting HTTP server (auto mode fallback)", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTP server (auto) failed: %w", err)
			}
		default:
			logger.Info("Starting HTTPS server", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTPS server failed: %w", err)
			}
		}
	}()
	return errCh
}

func startAncillaryServers(cfg *config.AppConfig, rootHandler http.Handler, logger *zap.Logger) (*http.Server, *http3.Server, chan error) {
	var metricsSrv *http.Server
	var quicSrv *http3.Server
	errCh := make(chan error, 2)

	if cfg.Metrics.ListenAddr != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{
			Addr:         cfg.Metrics.ListenAddr,
			Handler:      metricsMux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		go func() {
			logger.Info("Starting metrics server", zap.String("addr", cfg.Metrics.ListenAddr))
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("metrics server failed: %w", err)
			}
		}()
	}

	if cfg.Server.EnableQUIC {
		quicSrv = &http3.Server{
			Addr:    cfg.Server.QUICListenAddr,
			Handler: rootHandler,
			TLSConfig: &tls.Config{
				NextProtos: []string{"h3"},
			},
		}
		go func() {
			logger.Info("Starting QUIC server",
				zap.String("addr", cfg.Server.QUICListenAddr),
				zap.String("protocol", "quic/http3"))
			if err := quicSrv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil {
				errCh <- fmt.Errorf("QUIC server failed: %w", err)
			}
		}()
	}

	return metricsSrv, quicSrv, errCh
}

func awaitShutdownSignal(cancel context.CancelFunc, ancillaryErrCh, mainErrCh <-chan error, logger *zap.Logger) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		return nil
	case err := <-ancillaryErrCh:
		logger.Error("Server startup failed", zap.Error(err))
		cancel()
		return err
	case err := <-mainErrCh:
		logger.Error("Server startup failed", zap.Error(err))
		cancel()
		return err
	}
}

func shutdownServers(ctx context.Context, srv *http.Server, metricsSrv *http.Server, quicSrv *http3.Server, logger *zap.Logger) error {
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
		return err
	}

	var shutdownErr error
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			logger.Error("Metrics server forced to shutdown", zap.Error(err))
			shutdownErr = err
		}
	}

	if quicSrv != nil {
		if err := quicSrv.Close(); err != nil {
			logger.Error("QUIC server forced to shutdown", zap.Error(err))
			if shutdownErr == nil {
				shutdownErr = err
			}
		}
	}

	return shutdownErr
}

// runServer starts the CallFS server
func runServer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := initializeLogger(cfg.Log)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("Starting CallFS server",
		zap.String("instance_id", cfg.InstanceDiscovery.InstanceID),
		zap.String("listen_addr", cfg.Server.ListenAddr))

	// Initialize metadata store
	logger.Info("Initializing metadata store")
	metadataStore, raftMetadataStore, err := initMetadataStore(&cfg, logger)
	if err != nil {
		return err
	}
	defer metadataStore.Close()

	// Initialize distributed lock manager
	logger.Info("Initializing distributed lock manager")
	lockManager, err := initLockManager(&cfg, logger)
	if err != nil {
		return err
	}
	defer lockManager.Close()

	// Initialize backend adapters
	logger.Info("Initializing backend adapters")
	localFSBackend, s3Backend, internalProxyBackend, internalProxyAdapter, err := initBackends(&cfg, logger)
	if err != nil {
		return err
	}
	defer localFSBackend.Close()
	defer s3Backend.Close()
	defer internalProxyBackend.Close()

	// Initialize core engine
	logger.Info("Initializing core engine")
	coreEngine := core.NewEngine(
		metadataStore,
		localFSBackend,
		s3Backend,
		internalProxyBackend,
		internalProxyAdapter,
		lockManager,
		cfg.InstanceDiscovery.InstanceID,
		cfg.InstanceDiscovery.PeerEndpoints,
		cfg.HA.ReplicationEnabled,
		cfg.HA.ReplicaBackend,
		cfg.HA.RequireReplicaSuccess,
		logger)
	defer coreEngine.Close()

	// Initialize erasure manager if enabled
	if cfg.Erasure.Enabled {
		logger.Info("Initializing erasure coding manager")
		em, emErr := initErasureManager(&cfg, metadataStore, localFSBackend, s3Backend, logger)
		if emErr != nil {
			return emErr
		}
		defer em.Close()
		coreEngine.SetErasureManager(em)
		logger.Info("Erasure coding manager initialized",
			zap.Int("data_shards", cfg.Erasure.DataShards),
			zap.Int("parity_shards", cfg.Erasure.ParityShards))
	}

	// Ensure root directory exists in metadata store
	logger.Info("Ensuring root directory exists")
	if raftMetadataStore != nil {
		ensureRaftRootDirectory(&cfg, raftMetadataStore, coreEngine, logger)
	} else {
		if err := coreEngine.EnsureRootDirectory(context.Background()); err != nil {
			logger.Fatal("Failed to ensure root directory exists", zap.Error(err))
		}
	}

	rootHandler, routerResources, err := buildHTTPHandler(ctx, &cfg, coreEngine, metadataStore, localFSBackend, raftMetadataStore, logger)
	if err != nil {
		return err
	}
	defer routerResources.Stop()

	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      rootHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	metricsSrv, quicSrv, serverErrCh := startAncillaryServers(&cfg, rootHandler, logger)

	mainErrCh := startServer(srv, &cfg, logger)

	if err := awaitShutdownSignal(cancel, serverErrCh, mainErrCh, logger); err != nil {
		return err
	}

	logger.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := shutdownServers(shutdownCtx, srv, metricsSrv, quicSrv, logger); err != nil {
		return err
	}

	logger.Info("Server exited gracefully")
	return nil
}

// validateConfig validates the CallFS configuration and displays settings
func validateConfig(cmd *cobra.Command, args []string) error {
	fmt.Println("Validating configuration...")

	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		return err
	}

	fmt.Println("Configuration is valid")
	fmt.Printf("Instance ID: %s\n", cfg.InstanceDiscovery.InstanceID)
	fmt.Printf("Listen Address: %s\n", cfg.Server.ListenAddr)
	fmt.Printf("Metadata Store DSN: %s\n", maskDSN(cfg.MetadataStore.DSN))
	fmt.Printf("Redis Address: %s\n", cfg.DLM.RedisAddr)
	fmt.Printf("Local FS Root: %s\n", cfg.Backend.LocalFSRootPath)
	if cfg.Backend.S3BucketName != "" {
		fmt.Printf("S3 Bucket: %s\n", cfg.Backend.S3BucketName)
		fmt.Printf("S3 Region: %s\n", cfg.Backend.S3Region)
	}

	return nil
}

// maskDSN masks sensitive parts of the database DSN for display
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	// Very simple masking - in practice you'd want more sophisticated logic
	if len(dsn) > 20 {
		return dsn[:10] + "***" + dsn[len(dsn)-7:]
	}
	return "***"
}

// recoverMiddleware wraps an http.HandlerFunc with panic recovery and logging.
func recoverMiddleware(logger *zap.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				logger.Error("Internal handler panic",
					zap.Any("panic", rvr),
					zap.String("path", r.URL.Path))
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

// initializeLogger creates a zap logger based on configuration
func initializeLogger(logCfg config.LogConfig) (*zap.Logger, error) {
	var cfg zap.Config

	if logCfg.Format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch logCfg.Level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return cfg.Build()
}
