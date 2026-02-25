package main

//	@title		CallFS API
//	@version	1.0
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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/postgres"
	metadataraft "github.com/ebogdum/callfs/metadata/raft"
	metadataredis "github.com/ebogdum/callfs/metadata/redis"
	"github.com/ebogdum/callfs/metadata/schema"
	metadatasqlite "github.com/ebogdum/callfs/metadata/sqlite"
	"github.com/ebogdum/callfs/server"
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

func runClusterJoin(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err == nil {
		if strings.TrimSpace(joinNodeID) == "" {
			joinNodeID = strings.TrimSpace(cfg.Raft.NodeID)
		}
		if strings.TrimSpace(joinRaftAddr) == "" {
			joinRaftAddr = strings.TrimSpace(cfg.Raft.BindAddr)
		}
		if strings.TrimSpace(joinAPIEndpoint) == "" {
			joinAPIEndpoint = strings.TrimSpace(cfg.Server.ExternalURL)
		}
		if strings.TrimSpace(joinInternalSecret) == "" {
			joinInternalSecret = strings.TrimSpace(cfg.Auth.InternalProxySecret)
		}
	}

	joinNodeID = strings.TrimSpace(joinNodeID)
	joinRaftAddr = strings.TrimSpace(joinRaftAddr)
	joinAPIEndpoint = strings.TrimSpace(joinAPIEndpoint)
	joinInternalSecret = strings.TrimSpace(joinInternalSecret)

	if joinNodeID == "" {
		return fmt.Errorf("node id is required (use --node-id or set raft.node_id in config)")
	}
	if joinRaftAddr == "" {
		return fmt.Errorf("raft address is required (use --raft-addr or set raft.bind_addr in config)")
	}
	if joinAPIEndpoint == "" {
		return fmt.Errorf("api endpoint is required (use --api-endpoint or set server.external_url in config)")
	}
	if joinInternalSecret == "" {
		return fmt.Errorf("internal secret is required (use --internal-secret or set auth.internal_proxy_secret in config)")
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

// runServer starts the CallFS server
func runServer(cmd *cobra.Command, args []string) error {
	// Create context for the entire server lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger
	logger, err := initializeLogger(cfg.Log)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			// Log to stderr since logger may not be working
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("Starting CallFS server",
		zap.String("instance_id", cfg.InstanceDiscovery.InstanceID),
		zap.String("listen_addr", cfg.Server.ListenAddr))

	// Initialize metadata store
	logger.Info("Initializing metadata store")
	var metadataStore metadata.Store
	var raftMetadataStore *metadataraft.Store
	metadataStoreType := strings.ToLower(strings.TrimSpace(cfg.MetadataStore.Type))
	switch metadataStoreType {
	case "raft":
		apiPeers := make(map[string]string, len(cfg.Raft.APIPeerEndpoints)+1)
		for nodeID, endpoint := range cfg.Raft.APIPeerEndpoints {
			apiPeers[nodeID] = endpoint
		}
		if _, exists := apiPeers[cfg.Raft.NodeID]; !exists {
			apiPeers[cfg.Raft.NodeID] = cfg.Server.ExternalURL
		}

		store, storeErr := metadataraft.NewRaftStore(metadataraft.Config{
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
		if storeErr != nil {
			return fmt.Errorf("failed to initialize raft metadata store: %w", storeErr)
		}
		raftMetadataStore = store
		metadataStore = store
	case "sqlite":
		store, storeErr := metadatasqlite.NewSQLiteStore(cfg.MetadataStore.SQLitePath, logger)
		if storeErr != nil {
			return fmt.Errorf("failed to initialize sqlite metadata store: %w", storeErr)
		}
		metadataStore = store
	case "redis":
		store, storeErr := metadataredis.NewRedisStore(
			cfg.MetadataStore.RedisAddr,
			cfg.MetadataStore.RedisPassword,
			cfg.MetadataStore.RedisDB,
			cfg.MetadataStore.RedisKeyPrefix,
			logger,
		)
		if storeErr != nil {
			return fmt.Errorf("failed to initialize redis metadata store: %w", storeErr)
		}
		metadataStore = store
	case "postgres":
		logger.Info("Running database migrations")
		if err := schema.RunMigrations(cfg.MetadataStore.DSN); err != nil {
			return fmt.Errorf("failed to run database migrations: %w", err)
		}

		store, storeErr := postgres.NewPostgresStore(cfg.MetadataStore.DSN, logger)
		if storeErr != nil {
			return fmt.Errorf("failed to initialize postgres metadata store: %w", storeErr)
		}
		metadataStore = store
	default:
		return fmt.Errorf("unsupported metadata store type: %s", cfg.MetadataStore.Type)
	}
	defer metadataStore.Close()

	// Initialize distributed lock manager
	logger.Info("Initializing distributed lock manager")
	var lockManager locks.Manager
	dlmType := strings.ToLower(strings.TrimSpace(cfg.DLM.Type))
	switch dlmType {
	case "local":
		lockManager = locks.NewLocalManager()
	case "redis":
		manager, managerErr := locks.NewRedisManager(cfg.DLM.RedisAddr, cfg.DLM.RedisPassword, logger)
		if managerErr != nil {
			return fmt.Errorf("failed to initialize redis lock manager: %w", managerErr)
		}
		lockManager = manager
	default:
		return fmt.Errorf("unsupported dlm type: %s", cfg.DLM.Type)
	}
	defer lockManager.Close()

	// Initialize backend adapters conditionally
	logger.Info("Initializing backend adapters")

	// Initialize LocalFS backend if root path is configured
	var localFSBackend backends.Storage
	if cfg.Backend.LocalFSRootPath != "" {
		logger.Info("Initializing LocalFS backend", zap.String("root_path", cfg.Backend.LocalFSRootPath))
		backend, err := localfs.NewLocalFSAdapter(cfg.Backend.LocalFSRootPath)
		if err != nil {
			return fmt.Errorf("failed to initialize LocalFS backend: %w", err)
		}
		localFSBackend = backend
		defer localFSBackend.Close()
	} else {
		logger.Info("LocalFS backend disabled (no root path configured)")
		localFSBackend = noop.NewNoopAdapter()
	}

	// Initialize S3 backend if bucket name is configured
	var s3Backend backends.Storage
	if cfg.Backend.S3BucketName != "" {
		logger.Info("Initializing S3 backend", zap.String("bucket", cfg.Backend.S3BucketName))
		backend, err := s3.NewS3Adapter(cfg.Backend, logger)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}
		s3Backend = backend
		defer s3Backend.Close()
	} else {
		logger.Info("S3 backend disabled (no bucket configured)")
		s3Backend = noop.NewNoopAdapter()
	}

	// Initialize internal proxy backend if peer endpoints are configured
	var internalProxyBackend backends.Storage
	var internalProxyAdapter *internalproxy.InternalProxyAdapter
	if len(cfg.InstanceDiscovery.PeerEndpoints) > 0 {
		logger.Info("Initializing internal proxy backend", zap.Int("peer_count", len(cfg.InstanceDiscovery.PeerEndpoints)))
		adapter, err := internalproxy.NewInternalProxyAdapter(
			cfg.InstanceDiscovery.PeerEndpoints,
			cfg.Auth.InternalProxySecret,
			cfg.Backend.InternalProxySkipTLSVerify,
			logger)
		if err != nil {
			return fmt.Errorf("failed to initialize internal proxy backend: %w", err)
		}
		internalProxyAdapter = adapter
		internalProxyBackend = adapter
		defer internalProxyBackend.Close()
	} else {
		logger.Info("Internal proxy backend disabled (no peers configured)")
		internalProxyBackend = noop.NewNoopAdapter()
		internalProxyAdapter = nil
	}

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

	// Ensure root directory exists in metadata store
	logger.Info("Ensuring root directory exists")
	if raftMetadataStore != nil {
		if cfg.Raft.Bootstrap {
			waitDeadline := time.Now().Add(8 * time.Second)
			for !raftMetadataStore.IsLeader() && time.Now().Before(waitDeadline) {
				time.Sleep(200 * time.Millisecond)
			}
		}

		if raftMetadataStore.IsLeader() {
			if err := coreEngine.EnsureRootDirectory(context.Background()); err != nil {
				logger.Fatal("Failed to ensure root directory exists", zap.Error(err))
			}
		} else {
			logger.Info("Skipping root directory bootstrap on follower node",
				zap.String("node_id", cfg.Raft.NodeID),
				zap.String("leader_id", raftMetadataStore.LeaderID()))
		}
	} else {
		if err := coreEngine.EnsureRootDirectory(context.Background()); err != nil {
			logger.Fatal("Failed to ensure root directory exists", zap.Error(err))
		}
	}

	// Initialize authentication and authorization
	logger.Info("Initializing authentication and authorization")
	authenticator := auth.NewAPIKeyAuthenticator(cfg.Auth.APIKeys, cfg.Auth.InternalProxySecret)
	authorizer := auth.NewUnixAuthorizer(metadataStore)

	// Initialize link manager
	logger.Info("Initializing link manager")
	linkManager, err := links.NewLinkManager(metadataStore, cfg.Auth.SingleUseLinkSecret, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize link manager: %w", err)
	}

	// Start background cleanup worker
	links.StartCleanupWorker(ctx, metadataStore, 5*time.Minute, logger)

	// Initialize HTTP router
	logger.Info("Initializing HTTP router")
	router := server.NewRouter(coreEngine, authenticator, authorizer, linkManager, &cfg.Server, &cfg.Backend, cfg.Server.ExternalURL, logger)
	rootHandler := http.Handler(router)

	if raftMetadataStore != nil {
		mux := http.NewServeMux()
		mux.Handle("/", router)
		mux.HandleFunc("/v1/internal/raft/join", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			authHeader := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
			if authHeader != cfg.Auth.InternalProxySecret {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: "unauthorized"})
				return
			}

			if !raftMetadataStore.IsLeader() {
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: "not leader", LeaderID: raftMetadataStore.LeaderID()})
				return
			}

			var req metadataraft.JoinRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: fmt.Sprintf("invalid request: %v", err)})
				return
			}

			if err := raftMetadataStore.AddVoter(r.Context(), req.NodeID, req.RaftAddr, req.APIEndpoint); err != nil {
				status := http.StatusBadGateway
				if strings.Contains(strings.ToLower(err.Error()), "required") {
					status = http.StatusBadRequest
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "error", Error: err.Error(), LeaderID: raftMetadataStore.LeaderID()})
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(metadataraft.JoinResponse{Status: "joined", LeaderID: raftMetadataStore.LeaderID()})
		})
		mux.HandleFunc("/v1/internal/raft/metadata/apply", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			authHeader := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
			if authHeader != cfg.Auth.InternalProxySecret {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: "unauthorized"})
				return
			}

			var req metadataraft.ForwardApplyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: fmt.Sprintf("invalid request: %v", err)})
				return
			}

			res, err := raftMetadataStore.ApplyForwardedCommand(r.Context(), req.Command)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				errCode := err.Error()
				if err == metadata.ErrNotFound {
					errCode = "not_found"
				}
				if err == metadata.ErrAlreadyExists {
					errCode = "already_exists"
				}
				_ = json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{Error: errCode})
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(metadataraft.ForwardApplyResponse{CleanupCount: res.CleanupCount}); err != nil {
				logger.Error("Failed to encode raft apply response", zap.Error(err))
			}
		})
		rootHandler = mux
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      rootHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	var metricsSrv *http.Server
	var quicSrv *http3.Server
	if cfg.Metrics.ListenAddr != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{
			Addr:    cfg.Metrics.ListenAddr,
			Handler: metricsMux,
		}

		go func() {
			logger.Info("Starting metrics server", zap.String("addr", cfg.Metrics.ListenAddr))
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start metrics server", zap.Error(err))
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
				logger.Fatal("Failed to start QUIC server", zap.Error(err))
			}
		}()
	}

	// Start server in a goroutine
	go func() {
		protocol := strings.ToLower(cfg.Server.Protocol)
		if protocol == "" {
			protocol = "https"
		}

		switch protocol {
		case "http":
			logger.Info("Starting HTTP server", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start HTTP server", zap.Error(err))
			}
		case "auto":
			if cfg.Server.CertFile != "" && cfg.Server.KeyFile != "" {
				logger.Info("Starting HTTPS server (auto mode)", zap.String("addr", cfg.Server.ListenAddr))
				if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
					logger.Fatal("Failed to start HTTPS server", zap.Error(err))
				}
				return
			}

			logger.Info("Starting HTTP server (auto mode fallback)", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start HTTP server", zap.Error(err))
			}
		default:
			logger.Info("Starting HTTPS server", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start HTTPS server", zap.Error(err))
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Create a deadline for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
		return err
	}

	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Metrics server forced to shutdown", zap.Error(err))
			return err
		}
	}

	if quicSrv != nil {
		if err := quicSrv.Close(); err != nil {
			logger.Error("QUIC server forced to shutdown", zap.Error(err))
			return err
		}
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
