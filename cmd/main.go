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
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	"github.com/ebogdum/callfs/metadata/postgres"
	"github.com/ebogdum/callfs/metadata/schema"
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

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	Long:  "Validate the CallFS configuration and display the loaded settings",
	RunE:  validateConfig,
}

var configFilePath string

func main() {
	// Add flags to server command
	serverCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")

	// Add subcommands
	configCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(serverCmd, configCmd)

	// If no command specified, default to server
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "server")
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Error: %v", err)
	}
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

	// Run database migrations
	logger.Info("Running database migrations")
	if err := schema.RunMigrations(cfg.MetadataStore.DSN); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Initialize metadata store
	logger.Info("Initializing metadata store")
	metadataStore, err := postgres.NewPostgresStore(cfg.MetadataStore.DSN, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize metadata store: %w", err)
	}
	defer metadataStore.Close()

	// Initialize distributed lock manager
	logger.Info("Initializing distributed lock manager")
	lockManager, err := locks.NewRedisManager(cfg.DLM.RedisAddr, cfg.DLM.RedisPassword, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize lock manager: %w", err)
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
		logger)

	// Ensure root directory exists in metadata store
	logger.Info("Ensuring root directory exists")
	if err := coreEngine.EnsureRootDirectory(context.Background()); err != nil {
		logger.Fatal("Failed to ensure root directory exists", zap.Error(err))
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

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting HTTPS server", zap.String("addr", cfg.Server.ListenAddr))
		if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
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

	logger.Info("Server exited gracefully")
	return nil
}

// validateConfig validates the CallFS configuration and displays settings
func validateConfig(cmd *cobra.Command, args []string) error {
	fmt.Println("Validating configuration...")

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("❌ Configuration validation failed: %v\n", err)
		return err
	}

	fmt.Println("✅ Configuration is valid")
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
