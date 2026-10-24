package main

//	@title		CallFS API
//	@version	1.6.0
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
//	@description				Type "Bearer" followed by a space and a static API key.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/backends/internalproxy"
	"github.com/ebogdum/callfs/backends/localfs"
	"github.com/ebogdum/callfs/backends/noop"
	"github.com/ebogdum/callfs/backends/s3"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/health"
	"github.com/ebogdum/callfs/internal/buildinfo"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/natsmeta"
	"github.com/ebogdum/callfs/metadata/postgres"
	metadataredis "github.com/ebogdum/callfs/metadata/redis"
	"github.com/ebogdum/callfs/metadata/schema"
	metadatasqlite "github.com/ebogdum/callfs/metadata/sqlite"
	"github.com/ebogdum/callfs/natsrv"
	"github.com/ebogdum/callfs/server"
	"github.com/ebogdum/callfs/server/admin"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/server/middleware"
	"github.com/ebogdum/callfs/server/nodes"
	"github.com/ebogdum/callfs/server/webui"
	"github.com/ebogdum/callfs/telemetry"
	"github.com/ebogdum/callfs/web"
)

var rootCmd = &cobra.Command{
	Use:   "callfs",
	Short: "CallFS - Ultra-lightweight REST API filesystem",
	Long: `CallFS is an ultra-lightweight, high-performance REST API filesystem 
that provides precise Linux filesystem semantics over various backends.`,
}

var serverCmd = &cobra.Command{
	Use:           "server",
	Short:         "Start the CallFS server",
	Long:          "Start the CallFS server with the configured backends and API endpoints",
	RunE:          runServer,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
}

var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Check that the local server answers /health",
	RunE:  runHealthcheck,
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	Long:  "Validate the CallFS configuration and display the loaded settings",
	RunE:  validateConfig,
}

var configFilePath string
var joinInternalSecret string

const exitCodeRestart = 75

var errRestartRequested = errors.New("restart requested through the admin API")

func main() {
	// Add flags to server command
	serverCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	configCmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	healthcheckCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to configuration file")
	configCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(serverCmd, configCmd, healthcheckCmd)

	// If no command specified, default to server
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "server")
	}

	if err := rootCmd.Execute(); nil != err {
		if errors.Is(err, errRestartRequested) {
			os.Exit(exitCodeRestart)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runHealthcheck(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfigFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	host, port, err := net.SplitHostPort(cfg.Server.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid server.listen_addr %q: %w", cfg.Server.ListenAddr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}

	scheme := "http"
	client := &http.Client{Timeout: 5 * time.Second}
	if cfg.Server.CertFile != "" && cfg.Server.KeyFile != "" {
		scheme = "https"
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: host == "localhost",
			},
		}
	}

	healthURL := fmt.Sprintf("%s://%s/health", scheme, net.JoinHostPort(host, port))
	resp, err := client.Get(healthURL)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func buildRedisTLSConfig(enabled bool, caFile string, skipVerify bool) (*tls.Config, error) {
	if !enabled {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skipVerify,
	}
	if "" == caFile {
		return tlsConfig, nil
	}
	caPEM, err := os.ReadFile(caFile)
	if nil != err {
		return nil, fmt.Errorf("failed to read redis TLS CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("redis TLS CA file contains no valid certificates")
	}
	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}

func initMetadataStore(ctx context.Context, cfg *config.AppConfig, natsServer *natsrv.Server, logger *zap.Logger) (metadata.Store, error) {
	metadataStoreType := strings.ToLower(strings.TrimSpace(cfg.MetadataStore.Type))
	switch metadataStoreType {
	case "nats":
		if nil == natsServer {
			return nil, errors.New("metadata_store.type=nats requires nats.enabled=true")
		}
		store, err := natsmeta.Open(ctx, natsServer.Conn(), natsServer.JetStream(), cfg.NATS.ServerName, cfg.NATS.Metadata, logger)
		if nil != err {
			return nil, fmt.Errorf("failed to initialize nats metadata store: %w", err)
		}
		store.SetStrictReads(cfg.Performance.Strict())
		return store, nil
	case "sqlite":
		store, err := metadatasqlite.NewSQLiteStore(cfg.MetadataStore.SQLitePath, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize sqlite metadata store: %w", err)
		}
		return store, nil
	case "redis":
		tlsConfig, err := buildRedisTLSConfig(cfg.MetadataStore.RedisTLS, cfg.MetadataStore.RedisTLSCAFile, cfg.MetadataStore.RedisTLSSkipVerify)
		if nil != err {
			return nil, fmt.Errorf("failed to build redis TLS config: %w", err)
		}
		store, err := metadataredis.NewRedisStoreWithOptions(
			cfg.MetadataStore.RedisAddr,
			cfg.MetadataStore.RedisUsername,
			cfg.MetadataStore.RedisPassword,
			cfg.MetadataStore.RedisDB,
			cfg.MetadataStore.RedisKeyPrefix,
			tlsConfig,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize redis metadata store: %w", err)
		}
		return store, nil
	case "postgres":
		logger.Info("Running database migrations")
		migrationDSN := cfg.MetadataStore.MigrationDSN
		if "" == migrationDSN {
			migrationDSN = cfg.MetadataStore.DSN
		}
		if err := schema.RunMigrations(migrationDSN); err != nil {
			return nil, fmt.Errorf("failed to run database migrations: %w", err)
		}
		store, err := postgres.NewPostgresStoreWithPool(
			cfg.MetadataStore.DSN,
			cfg.MetadataStore.Postgres.MaxOpenConns,
			cfg.MetadataStore.Postgres.MaxIdleConns,
			cfg.MetadataStore.Postgres.ConnMaxLifetime,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize postgres metadata store: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported metadata store type: %s", cfg.MetadataStore.Type)
	}
}

func initLockManager(ctx context.Context, cfg *config.AppConfig, natsServer *natsrv.Server, logger *zap.Logger) (locks.Manager, error) {
	dlmType := strings.ToLower(strings.TrimSpace(cfg.DLM.Type))
	switch dlmType {
	case "nats":
		if nil == natsServer {
			return nil, errors.New("dlm.type=nats requires nats.enabled=true")
		}
		manager, err := locks.NewNATSManager(ctx, natsServer.JetStream(), cfg.NATS.ServerName, cfg.NATS.Locks.TTL, logger)
		if nil != err {
			return nil, fmt.Errorf("failed to initialize nats lock manager: %w", err)
		}
		manager.SetWaitTimeout(cfg.DLM.WaitTimeout)
		return manager, nil
	case "local":
		manager := locks.NewLocalManager()
		manager.SetWaitTimeout(cfg.DLM.WaitTimeout)
		return manager, nil
	case "redis":
		tlsConfig, err := buildRedisTLSConfig(cfg.DLM.RedisTLS, cfg.DLM.RedisTLSCAFile, cfg.DLM.RedisTLSSkipVerify)
		if nil != err {
			return nil, fmt.Errorf("failed to build dlm redis TLS config: %w", err)
		}
		manager, err := locks.NewRedisManagerWithOptions(locks.RedisOptions{
			Addr:         cfg.DLM.RedisAddr,
			Password:     cfg.DLM.RedisPassword,
			PoolSize:     cfg.DLM.RedisPoolSize,
			MinIdleConns: cfg.DLM.RedisMinIdleConns,
			TLSConfig:    tlsConfig,
		}, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize redis lock manager: %w", err)
		}
		manager.SetWaitTimeout(cfg.DLM.WaitTimeout)
		return manager, nil
	default:
		return nil, fmt.Errorf("unsupported dlm type: %s", cfg.DLM.Type)
	}
}

func initBackends(cfg *config.AppConfig, logger *zap.Logger) (localFS, s3Back, proxyBack backends.Storage, proxyAdapter *internalproxy.InternalProxyAdapter, err error) {
	if cfg.Backend.LocalFSRootPath != "" {
		logger.Info("Initializing LocalFS backend", zap.String("root_path", cfg.Backend.LocalFSRootPath))
		ack, ackErr := backends.ParseWriteAck(cfg.Backend.LocalFSAck)
		if nil != ackErr {
			return nil, nil, nil, nil, ackErr
		}
		localFS, err = localfs.NewLocalFSAdapter(cfg.Backend.LocalFSRootPath,
			localfs.WithSyncMode(localfs.SyncMode(cfg.Backend.LocalFSSync)),
			localfs.WithWriteAck(ack),
			localfs.WithFlush(localfs.FlushConfig{Workers: cfg.Backend.FlushWorkers, MaxPendingBytes: cfg.Backend.FlushMaxPendingBytes, MaxDelay: cfg.Backend.FlushMaxDelay}))
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

func buildHTTPHandler(cfg *config.AppConfig, coreEngine *core.Engine, metadataStore metadata.Store, localFSBackend backends.Storage, lockManager locks.Manager, hub *telemetry.Hub, runner *jobs.Runner, restart func(reason string) error, logger *zap.Logger) (http.Handler, *server.RouterResources, error) {
	logger.Info("Initializing authentication and authorization")
	if len(cfg.Auth.APIKeys) > 0 {
		logger.Warn("auth.api_keys assigns user IDs by list position (api-user-N); removing or reordering a key reassigns those IDs and transfers ownership of existing files to the next key holder. Use auth.api_key_users to bind each key to a stable identity.",
			zap.Int("positional_key_count", len(cfg.Auth.APIKeys)))
	}
	authenticator := auth.NewAPIKeyAuthenticator(cfg.Auth.APIKeys, cfg.Auth.APIKeyUsers, cfg.Auth.InternalProxySecret)
	authorizer := auth.NewUnixAuthorizer(metadataStore, authenticator)
	admins := auth.NewAdminSet(cfg.Auth.AdminUsers)
	sessionKey, err := auth.DeriveSessionKey(cfg.Auth.InternalProxySecret)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive session key: %w", err)
	}
	sessions := auth.NewSessionManager(sessionKey, cfg.UI.SessionTTL)

	logger.Info("Initializing link manager")
	linkManager, err := links.NewLinkManager(metadataStore, cfg.Auth.SingleUseLinkSecret, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize link manager: %w", err)
	}

	rateLimits, err := newRateLimits(cfg, logger)
	if err != nil {
		return nil, nil, err
	}

	var trash *core.Trash
	if cfg.Trash.Enabled {
		trash = core.NewTrash(coreEngine, authorizer, admins, runner, cfg.Trash.Retention, logger)
	}

	uploads, err := core.NewUploads(coreEngine, authorizer, cfg.Backend.LocalFSRootPath, cfg.Backend.DefaultBackend, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize uploads: %w", err)
	}

	erasureRecords, isRecordStore := metadataStore.(server.ErasureRecordStore)
	if !isRecordStore && cfg.Erasure.Enabled {
		return nil, nil, errors.New("metadata store does not keep erasure records")
	}

	inspector, isInspector := lockManager.(locks.Inspector)
	if !isInspector {
		return nil, nil, errors.New("lock manager does not expose lock inspection")
	}

	sources, err := config.CaptureSources(configFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture configuration sources: %w", err)
	}

	sampler, err := telemetry.NewSampler(telemetry.SamplerConfig{
		Gatherer:  prometheus.DefaultGatherer,
		Interval:  cfg.UI.MetricsSampleInterval,
		Retention: cfg.UI.MetricsRetention,
		Declared:  telemetry.UIMetricFamilies,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize metrics sampler: %w", err)
	}

	checker := buildHealthChecker(cfg, coreEngine, metadataStore, lockManager, hub, runner)

	api := admin.NewAPI(admin.Deps{
		Engine:        coreEngine,
		Store:         metadataStore,
		Erasure:       coreEngine.GetErasureManager(),
		Config:        cfg,
		ConfigPath:    configFilePath,
		ConfigSources: sources,
		Authenticator: authenticator,
		Admins:        admins,
		Links:         linkManager,
		RateLimits:    rateLimits,
		Locks:         inspector,
		Telemetry:     hub,
		Sampler:       sampler,
		Jobs:          runner,
		Trash:         trash,
		Uploads:       uploads,
		Health:        checker,
		Restart:       restart,
		Logger:        logger,
	})
	api.AttachFindings()

	if err := registerWorkers(runner, cfg, metadataStore, trash, uploads, api, sampler); err != nil {
		return nil, nil, err
	}

	logger.Info("Initializing HTTP router")
	nodeDirectory, err := nodes.New(nodes.Config{
		SelfID:         cfg.InstanceDiscovery.InstanceID,
		SelfEndpoint:   cfg.Server.ExternalURL,
		Peers:          clusterPeers(cfg),
		InternalSecret: cfg.Auth.InternalProxySecret,
		User:           middleware.GetUserID,
		IsAdmin:        admins.IsAdmin,
		Logger:         logger,
	})
	if nil != err {
		return nil, nil, fmt.Errorf("failed to set up cluster node directory: %w", err)
	}
	router, routerResources := server.NewRouterWithOptions(server.RouterOptions{
		Engine:        coreEngine,
		Authenticator: authenticator,
		Authorizer:    authorizer,
		Links:         linkManager,
		ServerConfig:  &cfg.Server,
		BackendConfig: &cfg.Backend,
		APIHost:       cfg.Server.ExternalURL,
		Logger:        logger,
		Config:        cfg,
		UI:            cfg.UI,
		Store:         metadataStore,
		ErasureStore:  erasureRecords,
		Erasure:       coreEngine.GetErasureManager(),
		Locks:         inspector,
		Sessions:      sessions,
		Admins:        admins,
		RateLimits:    rateLimits,
		Telemetry:     hub,
		Jobs:          runner,
		Trash:         trash,
		Uploads:       uploads,
		Health:        checker,
		Admin:         api,
		Nodes:         nodeDirectory,
	})
	internalMux := http.NewServeMux()

	if cfg.Erasure.Enabled {
		registerErasureRoutes(internalMux, localFSBackend, cfg.Auth.InternalProxySecret, logger)
	}

	handler, err := webui.Mount(routeInternalPaths(internalMux, router), web.Assets, web.Embedded, cfg.UI.Enabled, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to mount web console: %w", err)
	}
	return handler, routerResources, nil
}

type eventPublisher struct {
	bus    *telemetry.EventBus
	logger *zap.Logger
}

func (p eventPublisher) Publish(eventType string, payload any) {
	event := telemetry.Event{
		Type:    eventType,
		Level:   telemetry.LevelInfo,
		Message: eventType,
		Fields:  map[string]any{"payload": payload},
	}
	if job, ok := payload.(jobs.Job); ok {
		event.Entities = []telemetry.Entity{{Kind: "job", ID: job.ID}}
	}
	if err := p.bus.Publish(event); err != nil {
		p.logger.Warn("Failed to publish job event", zap.String("type", eventType), zap.Error(err))
	}
}

func newRateLimits(cfg *config.AppConfig, logger *zap.Logger) (*middleware.RateLimitRegistry, error) {
	registry := middleware.NewRateLimitRegistry(logger)
	limits := []struct {
		endpoint string
		rps      float64
		burst    int
	}{
		{middleware.RateLimitDownload, cfg.Server.RateLimit.DownloadRPS, cfg.Server.RateLimit.DownloadBurst},
		{middleware.RateLimitLinkGenerate, cfg.Server.RateLimit.LinkGenerateRPS, cfg.Server.RateLimit.LinkGenerateBurst},
		{middleware.RateLimitSession, cfg.Server.RateLimit.SessionRPS, cfg.Server.RateLimit.SessionBurst},
	}
	for _, limit := range limits {
		if err := registry.SetLimits(limit.endpoint, limit.rps, limit.burst); err != nil {
			registry.Stop()
			return nil, fmt.Errorf("invalid %s rate limit: %w", limit.endpoint, err)
		}
	}
	return registry, nil
}

func buildHealthChecker(cfg *config.AppConfig, coreEngine *core.Engine, metadataStore metadata.Store, lockManager locks.Manager, hub *telemetry.Hub, runner *jobs.Runner) *health.Checker {
	return health.NewChecker([]health.Probe{
		health.FileIndexProbe(func(ctx context.Context) error {
			_, err := metadataStore.Get(ctx, "/")
			return err
		}),
		health.LocksProbe(lockManager, cfg.InstanceDiscovery.InstanceID),
		health.StorageProbe("storage.localfs", "" != cfg.Backend.LocalFSRootPath, storageCheck(coreEngine, "localfs")),
		health.StorageProbe("storage.s3", "" != cfg.Backend.S3BucketName, storageCheck(coreEngine, "s3")),
		health.StorageProbe("storage.internal_proxy", 0 < len(clusterPeers(cfg)), storageCheck(coreEngine, "internal_proxy")),
		health.ErasureProbe(cfg.Erasure.Enabled, nil != coreEngine.GetErasureManager()),
		health.TLSProbe(cfg.Server.Protocol, cfg.Server.CertFile, time.Now),
		health.DiskProbe(cfg.Backend.LocalFSRootPath, diskSpace),
		health.ReplicationProbe(cfg.HA.ReplicationEnabled, func() int {
			return hub.Replication.CountSince(time.Now().Add(-time.Hour))
		}),
		health.LinkCleanupProbe(linkCleanupLastRun(runner)),
	})
}

func storageCheck(engine *core.Engine, name string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		probe, err := engine.ProbeStorage(ctx, name)
		if err != nil {
			return err
		}
		if probe.OK {
			return nil
		}
		if "" == probe.Error {
			return fmt.Errorf("%s storage probe failed", name)
		}
		return errors.New(probe.Error)
	}
}

func linkCleanupLastRun(runner *jobs.Runner) func() (time.Time, error) {
	return func() (time.Time, error) {
		for _, status := range runner.Workers() {
			if "link_cleanup" != status.Name {
				continue
			}
			if nil != status.LastError {
				return time.Time{}, errors.New(*status.LastError)
			}
			if nil == status.LastRunAt {
				return time.Time{}, nil
			}
			return *status.LastRunAt, nil
		}
		return time.Time{}, nil
	}
}

func diskSpace(path string) (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to read disk space for %s: %w", path, err)
	}
	blockSize := uint64(stat.Bsize)
	return uint64(stat.Bavail) * blockSize, uint64(stat.Blocks) * blockSize, nil
}

const linkRetention = 24 * time.Hour

func registerWorkers(runner *jobs.Runner, cfg *config.AppConfig, metadataStore metadata.Store, trash *core.Trash, uploads *core.Uploads, api *admin.API, sampler *telemetry.Sampler) error {
	type scheduledWorker struct {
		name     string
		interval time.Duration
		run      jobs.WorkerFunc
	}
	workers := []scheduledWorker{
		{name: "link_cleanup", interval: 5 * time.Minute, run: linkCleanupWorker(metadataStore)},
		{name: "upload_cleanup", interval: 10 * time.Minute, run: uploadCleanupWorker(uploads)},
		{name: "instance_prober", interval: 30 * time.Second, run: api.InstanceProberWorker},
		{name: "metrics_sampler", interval: cfg.UI.MetricsSampleInterval, run: metricsSamplerWorker(sampler)},
		{name: "usage_refresh", interval: 10 * time.Minute, run: api.UsageRefreshWorker},
	}
	if nil != trash {
		workers = append(workers, scheduledWorker{name: "trash_cleanup", interval: 10 * time.Minute, run: trashCleanupWorker(trash, cfg.Trash.Retention)})
	}
	for _, w := range workers {
		if err := runner.RegisterWorker(w.name, w.interval, w.run); err != nil {
			return fmt.Errorf("failed to register worker %s: %w", w.name, err)
		}
	}
	return nil
}

func primeGauges(ctx context.Context, runner *jobs.Runner, store metadata.Store, logger *zap.Logger) {
	if _, err := runner.RunNow("usage_refresh", "callfs"); nil != err {
		logger.Warn("Startup usage refresh did not start", zap.Error(err))
	}
	if _, err := links.RefreshActiveLinks(ctx, store, time.Now()); nil != err {
		logger.Warn("Startup active link count failed", zap.Error(err))
	}
}

func linkCleanupWorker(store metadata.Store) jobs.WorkerFunc {
	return func(ctx context.Context) (any, error) {
		cutoff := time.Now().Add(-linkRetention)
		expired, expiredErr := store.CleanupExpiredLinks(ctx, cutoff)
		used, usedErr := store.CleanupUsedLinks(ctx, cutoff)
		active, activeErr := links.RefreshActiveLinks(ctx, store, time.Now())
		if err := errors.Join(expiredErr, usedErr, activeErr); err != nil {
			return nil, fmt.Errorf("link cleanup failed: %w", err)
		}
		return map[string]int{"expired_removed": expired, "used_removed": used, "active": active}, nil
	}
}

func uploadCleanupWorker(uploads *core.Uploads) jobs.WorkerFunc {
	return func(_ context.Context) (any, error) {
		removed, err := uploads.Cleanup(time.Now())
		if err != nil {
			return nil, fmt.Errorf("upload cleanup failed: %w", err)
		}
		return map[string]int{"removed": removed}, nil
	}
}

func trashCleanupWorker(trash *core.Trash, retention time.Duration) jobs.WorkerFunc {
	return func(ctx context.Context) (any, error) {
		removed, err := trash.Cleanup(ctx, retention)
		if err != nil {
			return nil, fmt.Errorf("trash cleanup failed: %w", err)
		}
		return map[string]int{"removed": removed}, nil
	}
}

func metricsSamplerWorker(sampler *telemetry.Sampler) jobs.WorkerFunc {
	return func(_ context.Context) (any, error) {
		if err := sampler.Sample(time.Now()); err != nil {
			return nil, fmt.Errorf("metrics sampling failed: %w", err)
		}
		return nil, nil
	}
}

func routeInternalPaths(internalMux *http.ServeMux, router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/internal/") || strings.HasPrefix(r.URL.Path, "/internal/") {
			internalMux.ServeHTTP(w, r)
			return
		}
		router.ServeHTTP(w, r)
	})
}

func registerErasureRoutes(mux *http.ServeMux, localFSBackend backends.Storage, internalSecret string, logger *zap.Logger) {
	storeShard := handlers.InternalStoreShardHandler(localFSBackend, internalSecret, logger)
	getShard := handlers.InternalGetShardHandler(localFSBackend, internalSecret, logger)
	deleteShard := handlers.InternalDeleteShardHandler(localFSBackend, internalSecret, logger)
	mux.HandleFunc("/v1/internal/shards/", recoverMiddleware(logger, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			storeShard(w, r)
		case http.MethodGet:
			getShard(w, r)
		case http.MethodDelete:
			deleteShard(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func secretMatches(presented, expected string) bool {
	presentedDigest := sha256.Sum256([]byte(presented))
	expectedDigest := sha256.Sum256([]byte(expected))
	return hmac.Equal(presentedDigest[:], expectedDigest[:])
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
			if "" == cfg.Server.CertFile || "" == cfg.Server.KeyFile {
				errCh <- fmt.Errorf("protocol auto requires both cert_file and key_file; refusing to serve plaintext HTTP")
				return
			}
			logger.Info("Starting HTTPS server (auto mode)", zap.String("addr", cfg.Server.ListenAddr))
			if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTPS server (auto) failed: %w", err)
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
		if cfg.Metrics.Profiling {
			mountProfiling(metricsMux)
			metricsSrv.WriteTimeout = 0
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

func awaitShutdownSignal(cancel context.CancelFunc, ancillaryErrCh, mainErrCh <-chan error, restartCh <-chan string, logger *zap.Logger) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		return nil
	case reason := <-restartCh:
		logger.Warn("Restart requested through the admin API", zap.String("reason", reason))
		return errRestartRequested
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

	hub, err := telemetry.NewHub(telemetry.Config{
		InstanceID:   cfg.InstanceDiscovery.InstanceID,
		LogEntries:   cfg.UI.LogBufferEntries,
		AuditEntries: cfg.UI.AuditBufferEntries,
		EventEntries: cfg.UI.EventBufferEntries,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	logger, err := initializeLogger(cfg.Log, hub)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	zap.ReplaceGlobals(logger)
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("Starting CallFS server",
		zap.String("version", buildinfo.Version),
		zap.String("commit", buildinfo.Commit),
		zap.String("build_date", buildinfo.Date),
		zap.String("go_version", runtime.Version()),
		zap.String("instance_id", cfg.InstanceDiscovery.InstanceID),
		zap.String("listen_addr", cfg.Server.ListenAddr))

	var natsServer *natsrv.Server
	if cfg.NATS.Enabled {
		natsServer, err = startNATS(ctx, &cfg, logger)
		if nil != err {
			return err
		}
		defer func() {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			if stopErr := natsServer.Shutdown(stopCtx); nil != stopErr {
				logger.Warn("NATS shutdown did not finish cleanly", zap.Error(stopErr))
			}
		}()
	}

	// Initialize metadata store
	logger.Info("Initializing metadata store")
	metadataStore, err := initMetadataStore(ctx, &cfg, natsServer, logger)
	if err != nil {
		return err
	}
	defer metadataStore.Close()
	runner := jobs.NewRunner(eventPublisher{bus: hub.Events, logger: logger}, logger)

	// Initialize distributed lock manager
	logger.Info("Initializing distributed lock manager")
	lockManager, err := initLockManager(ctx, &cfg, natsServer, logger)
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
		metadata.Instrument(metadataStore),
		backends.Instrument("localfs", localFSBackend),
		backends.Instrument("s3", s3Backend),
		backends.Instrument("internalproxy", internalProxyBackend),
		internalProxyAdapter,
		lockManager,
		cfg.InstanceDiscovery.InstanceID,
		cfg.InstanceDiscovery.PeerEndpoints,
		cfg.HA.ReplicationEnabled,
		cfg.HA.ReplicaBackend,
		cfg.HA.RequireReplicaSuccess,
		logger)
	defer coreEngine.Close()
	coreEngine.SetStrict(cfg.Performance.Strict())
	coreEngine.SetMetadataCacheTTL(cfg.Server.MetadataCacheTTL)
	coreEngine.SetMetadataCacheSize(cfg.Server.MetadataCacheSize)

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
	if err := coreEngine.EnsureRootDirectory(context.Background()); nil != err {
		return fmt.Errorf("failed to ensure root directory exists: %w", err)
	}

	restartCh := make(chan string, 1)
	restart := func(reason string) error {
		select {
		case restartCh <- reason:
			return nil
		default:
			return errors.New("restart is already requested")
		}
	}

	rootHandler, routerResources, err := buildHTTPHandler(&cfg, coreEngine, metadataStore, localFSBackend, lockManager, hub, runner, restart, logger)
	if err != nil {
		return err
	}
	defer routerResources.Stop()

	if err := runner.Start(ctx); err != nil {
		return fmt.Errorf("failed to start background jobs: %w", err)
	}
	go primeGauges(ctx, runner, metadataStore, logger)

	srv := &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           withRequestDeadlines(rootHandler, &cfg.Server, logger),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	metricsSrv, quicSrv, serverErrCh := startAncillaryServers(&cfg, rootHandler, logger)

	mainErrCh := startServer(srv, &cfg, logger)

	stopErr := awaitShutdownSignal(cancel, serverErrCh, mainErrCh, restartCh, logger)
	if nil != stopErr && !errors.Is(stopErr, errRestartRequested) {
		return stopErr
	}

	logger.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := shutdownServers(shutdownCtx, srv, metricsSrv, quicSrv, logger); err != nil {
		return err
	}

	if nil != stopErr {
		return stopErr
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
	if "" == dsn {
		return ""
	}
	parsed, err := neturl.Parse(dsn)
	if nil != err || "" == parsed.Scheme || "" == parsed.Host {
		return "***"
	}
	return parsed.Scheme + "://" + parsed.Host
}

// recoverMiddleware wraps an http.HandlerFunc with panic recovery and logging.
func recoverMiddleware(logger *zap.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				logger.Error("Internal handler panic",
					zap.Any("panic", rvr),
					zap.String("path", middleware.RedactDownloadPath(r.URL.Path)))
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

// initializeLogger creates a zap logger based on configuration
func initializeLogger(logCfg config.LogConfig, hub *telemetry.Hub) (*zap.Logger, error) {
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

	encoder := zapcore.NewConsoleEncoder(cfg.EncoderConfig)
	if "json" == logCfg.Format {
		encoder = zapcore.NewJSONEncoder(cfg.EncoderConfig)
	}
	sink := &zapcore.BufferedWriteSyncer{WS: zapcore.Lock(os.Stderr), Size: logBufferBytes, FlushInterval: logFlushInterval}
	core := zapcore.NewCore(encoder, sink, cfg.Level)
	return zap.New(hub.Tee(core), zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)), nil
}

const (
	logBufferBytes   = 256 << 10
	logFlushInterval = time.Second
)

func clusterPeers(cfg *config.AppConfig) map[string]string {
	peers := make(map[string]string, len(cfg.InstanceDiscovery.PeerEndpoints))
	for id, endpoint := range cfg.InstanceDiscovery.PeerEndpoints {
		peers[id] = endpoint
	}
	return peers
}

func startNATS(ctx context.Context, cfg *config.AppConfig, logger *zap.Logger) (*natsrv.Server, error) {
	peers := clusterPeers(cfg)
	logger.Info("Starting embedded NATS", zap.String("server_name", cfg.NATS.ServerName), zap.Int("peers", len(peers)))
	natsServer, err := natsrv.Start(ctx, cfg.NATS, cfg.Auth.InternalProxySecret, peers, logger)
	if nil != err {
		return nil, fmt.Errorf("failed to start embedded NATS: %w", err)
	}
	replicas := min(len(peers)+1, max(cfg.NATS.Metadata.Replicas, cfg.NATS.Locks.Replicas, cfg.NATS.Streams.Replicas))
	if err := provisionNATS(ctx, natsServer, natsrv.DesiredTopology(cfg.NATS, replicas), logger); nil != err {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		return nil, errors.Join(fmt.Errorf("failed to provision NATS streams: %w", err), natsServer.Shutdown(stopCtx))
	}
	if err := prometheus.Register(natsrv.NewCollector(natsServer.Embedded())); nil != err {
		logger.Warn("NATS metrics collector not registered", zap.Error(err))
	}
	return natsServer, nil
}

func provisionNATS(ctx context.Context, natsServer *natsrv.Server, top natsrv.Topology, logger *zap.Logger) error {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 3*time.Second)
		err := natsrv.Provision(attemptCtx, natsServer.JetStream(), top)
		attemptCancel()
		if nil == err {
			return nil
		}
		if nil != ctx.Err() || !time.Now().Before(deadline) {
			return err
		}
		logger.Info("Waiting for the NATS cluster before creating streams", zap.Error(err))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
