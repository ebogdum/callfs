package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/time/rate"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/health"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/links"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
	"github.com/ebogdum/callfs/v2/server/admin"
	"github.com/ebogdum/callfs/v2/server/handlers"
	linksHandlers "github.com/ebogdum/callfs/v2/server/handlers/links"
	authMiddleware "github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/server/nodes"
	"github.com/ebogdum/callfs/v2/telemetry"
)

type RouterResources struct {
	rateLimiters []*authMiddleware.RateLimiterHandle
	rateLimits   *authMiddleware.RateLimitRegistry
}

func (rr *RouterResources) Stop() {
	for _, h := range rr.rateLimiters {
		h.Stop()
	}
	if nil != rr.rateLimits {
		rr.rateLimits.Stop()
	}
}

type ClusterRoleInfo interface {
	LeaderInfo() (state, leaderID, leaderAddr string)
}

type ErasureRecordStore interface {
	metadata.ErasureMetadataStore
	ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error)
}

type RouterOptions struct {
	Engine        *core.Engine
	Authenticator auth.Authenticator
	Authorizer    auth.Authorizer
	Links         *links.LinkManager
	ServerConfig  *config.ServerConfig
	BackendConfig *config.BackendConfig
	APIHost       string
	ClusterRole   ClusterRoleInfo
	Logger        *zap.Logger

	Config       *config.AppConfig
	UI           config.UIConfig
	Store        metadata.Store
	ErasureStore ErasureRecordStore
	Erasure      *erasure.Manager
	Locks        locks.Inspector
	Sessions     *auth.SessionManager
	Admins       *auth.AdminSet
	RateLimits   *authMiddleware.RateLimitRegistry
	Telemetry    *telemetry.Hub
	Jobs         *jobs.Runner
	Trash        *core.Trash
	Uploads      *core.Uploads
	Health       *health.Checker
	Admin        *admin.API
	Nodes        *nodes.Directory
}

type healthCluster struct {
	State      string `json:"state"`
	LeaderID   string `json:"leader_id"`
	LeaderAddr string `json:"leader_addr"`
}

type healthResponse struct {
	Status  string         `json:"status"`
	Cluster *healthCluster `json:"cluster,omitempty"`
}

func parseTrustedProxies(cidrs []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if nil != err {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", cidr, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func isTrustedProxyAddr(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func resolveClientIP(remoteHost string, r *http.Request, trustedProxies []netip.Prefix) string {
	remote, err := netip.ParseAddr(remoteHost)
	if nil != err || !isTrustedProxyAddr(remote.Unmap(), trustedProxies) {
		return remoteHost
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; 0 <= i; i-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if nil != err {
			break
		}
		if !isTrustedProxyAddr(candidate.Unmap(), trustedProxies) {
			return candidate.Unmap().String()
		}
	}
	if realIP, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); nil == err {
		return realIP.Unmap().String()
	}
	return remoteHost
}

func trustedClientIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if 0 == len(trustedProxies) {
				next.ServeHTTP(w, r)
				return
			}
			host, port, err := net.SplitHostPort(r.RemoteAddr)
			if nil != err {
				next.ServeHTTP(w, r)
				return
			}
			clone := new(http.Request)
			*clone = *r
			clone.RemoteAddr = net.JoinHostPort(resolveClientIP(host, r, trustedProxies), port)
			next.ServeHTTP(w, clone)
		})
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func metricsMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func clientHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if nil != err {
		return remoteAddr
	}
	return host
}

func requestMetricsMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			panicked := true
			defer func() {
				duration := time.Since(start)
				routePattern := chi.RouteContext(r.Context()).RoutePattern()
				if routePattern == "" {
					routePattern = "unmatched"
				}
				status := ww.Status()
				if panicked && 0 == status {
					status = http.StatusInternalServerError
				}
				hijacked := 0 == status && isWebSocketUpgrade(r)
				if hijacked {
					status = http.StatusSwitchingProtocols
				}
				metricMethod := metricsMethod(r.Method)
				requestCounter(metricMethod, routePattern, status).Inc()

				if http.StatusBadRequest <= status {
					errorCounter(routePattern, status).Inc()
				}

				streamed := strings.HasPrefix(ww.Header().Get("Content-Type"), "text/event-stream")
				if !hijacked && !streamed {
					durationObserver(metricMethod, routePattern).Observe(duration.Seconds())
				}

				if entry := logger.Check(zapcore.DebugLevel, "HTTP request"); nil != entry {
					entry.Write(
						zap.String("method", r.Method),
						zap.String("path", routePattern),
						zap.Int("status", status),
						zap.Duration("duration", duration),
						zap.String("user_agent", r.UserAgent()),
						zap.String("remote_addr", r.RemoteAddr))
				}
			}()

			next.ServeHTTP(ww, r)
			panicked = false
		})
	}
}

type requestCounterKey struct {
	method string
	route  string
	status int
}

type durationObserverKey struct {
	method string
	route  string
}

type errorCounterKey struct {
	route  string
	status int
}

var (
	requestCounters   sync.Map
	durationObservers sync.Map
	errorCounters     sync.Map
)

func requestCounter(method, route string, status int) prometheus.Counter {
	key := requestCounterKey{method: method, route: route, status: status}
	if cached, ok := requestCounters.Load(key); ok {
		return cached.(prometheus.Counter)
	}
	child := metrics.HTTPRequestsTotal.WithLabelValues(method, route, strconv.Itoa(status))
	actual, _ := requestCounters.LoadOrStore(key, child)
	return actual.(prometheus.Counter)
}

func durationObserver(method, route string) prometheus.Observer {
	key := durationObserverKey{method: method, route: route}
	if cached, ok := durationObservers.Load(key); ok {
		return cached.(prometheus.Observer)
	}
	child := metrics.HTTPRequestDuration.WithLabelValues(method, route)
	actual, _ := durationObservers.LoadOrStore(key, child)
	return actual.(prometheus.Observer)
}

func errorCounter(route string, status int) prometheus.Counter {
	key := errorCounterKey{route: route, status: status}
	if cached, ok := errorCounters.Load(key); ok {
		return cached.(prometheus.Counter)
	}
	child := metrics.ErrorsTotal.WithLabelValues(errorComponent(route), errorType(status))
	actual, _ := errorCounters.LoadOrStore(key, child)
	return actual.(prometheus.Counter)
}

var errorTypes = map[int]string{
	http.StatusBadRequest:                   "bad_request",
	http.StatusUnauthorized:                 "unauthorized",
	http.StatusForbidden:                    "forbidden",
	http.StatusNotFound:                     "not_found",
	http.StatusMethodNotAllowed:             "method_not_allowed",
	http.StatusRequestTimeout:               "timeout",
	http.StatusConflict:                     "conflict",
	http.StatusGone:                         "gone",
	http.StatusPreconditionFailed:           "precondition_failed",
	http.StatusRequestEntityTooLarge:        "too_large",
	http.StatusUnsupportedMediaType:         "unsupported_media_type",
	http.StatusRequestedRangeNotSatisfiable: "bad_range",
	http.StatusUnprocessableEntity:          "invalid",
	http.StatusTooManyRequests:              "rate_limited",
	http.StatusInternalServerError:          "internal",
	http.StatusNotImplemented:               "not_implemented",
	http.StatusBadGateway:                   "bad_gateway",
	http.StatusServiceUnavailable:           "unavailable",
	http.StatusGatewayTimeout:               "gateway_timeout",
	http.StatusInsufficientStorage:          "insufficient_storage",
}

func errorType(status int) string {
	if name, known := errorTypes[status]; known {
		return name
	}
	if http.StatusInternalServerError <= status {
		return "server_error"
	}
	return "client_error"
}

func errorComponent(routePattern string) string {
	parts := strings.Split(strings.Trim(routePattern, "/"), "/")
	component := parts[0]
	if "v1" == component && 1 < len(parts) {
		component = parts[1]
	}
	if "" == component || strings.ContainsAny(component, "*{") {
		return "other"
	}
	return component
}

func trackTransfer(kind string, next http.Handler) http.HandlerFunc {
	active := metrics.TransfersActive.WithLabelValues(kind)
	return func(w http.ResponseWriter, r *http.Request) {
		active.Inc()
		defer active.Dec()
		next.ServeHTTP(w, r)
	}
}

func healthEndpoint(roleInfo ClusterRoleInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := healthResponse{Status: "ok"}
		if nil != roleInfo {
			state, leaderID, leaderAddr := roleInfo.LeaderInfo()
			body.Cluster = &healthCluster{State: state, LeaderID: leaderID, LeaderAddr: leaderAddr}
		}
		payload, err := json.Marshal(body)
		if nil != err {
			http.Error(w, "failed to encode health response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(payload); nil != err {
			slog.Error("Failed to write health check response", "error", err)
		}
	}
}

func authFailureRecorder(hub *telemetry.Hub) func(reason string, r *http.Request) {
	return func(reason string, r *http.Request) {
		if nil == hub {
			return
		}
		hub.AuthFailures.Record(telemetry.AuthFailure{
			At:     time.Now(),
			Client: clientHost(r.RemoteAddr),
			Reason: reason,
			Path:   r.URL.Path,
		})
	}
}

func limitMiddleware(opts RouterOptions, resources *RouterResources, endpoint string, rps float64, burst int, logger *zap.Logger) (func(http.Handler) http.Handler, error) {
	if nil != opts.RateLimits {
		return opts.RateLimits.Middleware(endpoint)
	}
	handle, mw := authMiddleware.V1RateLimitMiddleware(rate.NewLimiter(rate.Limit(rps), burst), logger)
	resources.rateLimiters = append(resources.rateLimiters, handle)
	return mw, nil
}

func NewRouter(
	engine *core.Engine,
	authenticator auth.Authenticator,
	authorizer auth.Authorizer,
	linkManager *links.LinkManager,
	serverConfig *config.ServerConfig,
	backendConfig *config.BackendConfig,
	apiHost string,
	roleInfo ClusterRoleInfo,
	logger *zap.Logger,
) (chi.Router, *RouterResources) {
	var erasureManager *erasure.Manager
	if nil != engine {
		erasureManager = engine.GetErasureManager()
	}
	return NewRouterWithOptions(RouterOptions{
		Engine:        engine,
		Authenticator: authenticator,
		Authorizer:    authorizer,
		Links:         linkManager,
		ServerConfig:  serverConfig,
		BackendConfig: backendConfig,
		APIHost:       apiHost,
		ClusterRole:   roleInfo,
		Logger:        logger,
		Erasure:       erasureManager,
		Admins:        auth.NewAdminSet(nil),
	})
}

func NewRouterWithOptions(opts RouterOptions) (*chi.Mux, *RouterResources) {
	resources := &RouterResources{rateLimits: opts.RateLimits}
	logger := opts.Logger
	if nil == logger {
		logger = zap.NewNop()
	}
	fingerprints, _ := opts.Authenticator.(interface {
		UserForFingerprint(fingerprint string) (string, bool)
	})

	r := chi.NewRouter()

	trustedProxies, err := parseTrustedProxies(opts.ServerConfig.TrustedProxies)
	if nil != err {
		logger.Error("Invalid trusted proxy configuration; client IPs use connection address", zap.Error(err))
	}

	r.Use(authMiddleware.V1RequestIDMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(authMiddleware.V1SecurityHeaders())
	r.Use(trustedClientIPMiddleware(trustedProxies))
	r.Use(requestMetricsMiddleware(logger))

	r.Get("/health", healthEndpoint(opts.ClusterRole))
	registerHealthReady(r, opts.Health)

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.V1AuthMiddleware(opts.Authenticator, opts.Sessions, fingerprints, logger, authFailureRecorder(opts.Telemetry)))
		r.Handle("/metrics", promhttp.Handler())
	})

	r.Route("/v1", func(r chi.Router) {
		if nil != opts.Telemetry {
			r.Use(telemetry.AuditMiddleware(opts.Telemetry))
		}
		registerSessionRoute(r, opts, logger)
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.V1AuthMiddleware(opts.Authenticator, opts.Sessions, fingerprints, logger, authFailureRecorder(opts.Telemetry)))
			r.Use(writeAckMiddleware)
			registerAccountRoutes(r, opts)
			registerFileRoutes(r, opts, logger)
			registerLinkRoutes(r, opts, resources, logger)
			registerTrashRoutes(r, opts)
			registerUploadRoutes(r, opts)
			registerErasureRoutes(r, opts, logger)
			registerJobRoutes(r, opts)
			if nil != opts.Admin {
				admin.Mount(r, opts.Admin)
			}
			if nil != opts.Nodes {
				opts.Nodes.Mount(r)
			}
		})
	})

	registerDownloadRoutes(r, opts, resources, logger)

	if nil != opts.Nodes {
		opts.Nodes.SetLocal(r)
	}

	logger.Info("HTTP router configured successfully")

	return r, resources
}

func registerHealthReady(r chi.Router, checker *health.Checker) {
	if nil == checker {
		r.Get("/health/ready", handlers.HealthReady(nil))
		return
	}
	r.Get("/health/ready", handlers.HealthReady(checker))
}

func registerSessionRoute(r chi.Router, opts RouterOptions, logger *zap.Logger) {
	if nil == opts.Authenticator || nil == opts.Sessions || nil == opts.Admins || nil == opts.RateLimits {
		return
	}
	limit, err := opts.RateLimits.Middleware(authMiddleware.RateLimitSession)
	if nil != err {
		logger.Error("Session route not registered: session rate limit is not configured", zap.Error(err))
		return
	}
	r.With(limit).Post("/session", handlers.V1CreateSession(opts.Authenticator, opts.Sessions, opts.Admins, opts.UI, logger))
}

func registerAccountRoutes(r chi.Router, opts RouterOptions) {
	if nil != opts.Sessions {
		r.Delete("/session", handlers.V1DeleteSession())
	}
	if nil != opts.Admins {
		r.Get("/me", handlers.V1Me(opts.Admins, opts.Sessions))
	}
	if nil != opts.Telemetry {
		r.Get("/me/events", handlers.V1MyEvents(opts.Telemetry))
	}
	if nil != opts.Config {
		r.Get("/capabilities", handlers.V1Capabilities(handlers.CapabilitiesSource{
			Config:     opts.Config,
			InstanceID: opts.Config.InstanceDiscovery.InstanceID,
		}))
	}
	if nil != opts.Locks {
		r.Get("/stat/*", handlers.V1Stat(opts.Engine, opts.Authorizer, opts.Locks))
	}
	r.Get("/search", handlers.V1Search(opts.Engine, opts.Authorizer))
	r.Get("/permissions/check", handlers.V1PermissionCheck(opts.Authorizer, opts.Engine, opts.Admins))
	if nil != opts.Jobs {
		r.Get("/usage", handlers.V1Usage(opts.Engine, handlers.NewUsageCache(opts.Jobs)))
	}
}

func registerFileRoutes(r chi.Router, opts RouterOptions, logger *zap.Logger) {
	r.Route("/files", func(r chi.Router) {
		r.Get("/ws/*", trackTransfer("websocket", handlers.V1WebSocketTransfer(opts.Engine, opts.Authorizer, opts.BackendConfig, logger)))
		r.Get("/*", trackTransfer("download", handlers.V1GetFile(opts.Engine, opts.Authorizer, opts.ServerConfig, logger)))
		r.Head("/*", handlers.V1HeadFileEnhanced(opts.Engine, opts.Authorizer, logger))
		r.Post("/*", trackTransfer("upload", handlers.V1PostFileEnhanced(opts.Engine, opts.Authorizer, opts.BackendConfig, opts.ServerConfig, logger)))
		r.Put("/*", trackTransfer("upload", handlers.V1PutFileEnhanced(opts.Engine, opts.Authorizer, opts.BackendConfig, opts.ServerConfig, logger)))
		r.Patch("/*", handlers.V1PatchFileEnhanced(opts.Engine, opts.Authorizer, opts.Admins, opts.BackendConfig, opts.ServerConfig, logger))
		r.Delete("/*", handlers.V1DeleteFileEnhanced(opts.Engine, opts.Authorizer, logger))
	})

	if nil != opts.Erasure {
		r.Route("/shards", func(r chi.Router) {
			r.Get("/*", handlers.V1GetShard(opts.Erasure, opts.Authorizer, logger))
		})
	}

	r.Route("/directories", func(r chi.Router) {
		r.Get("/*", handlers.V1ListDirectoryWithConfig(opts.Engine, opts.Authorizer, opts.ServerConfig, logger))
	})

	r.Get("/objects", handlers.V1ListObjects(opts.Engine, opts.Authorizer, logger))
	r.Delete("/objects", handlers.V1DeleteObjects(opts.Engine, opts.Authorizer, logger))
}

func registerLinkRoutes(r chi.Router, opts RouterOptions, resources *RouterResources, logger *zap.Logger) {
	generateLimit, err := limitMiddleware(opts, resources, authMiddleware.RateLimitLinkGenerate, opts.ServerConfig.RateLimit.LinkGenerateRPS, opts.ServerConfig.RateLimit.LinkGenerateBurst, logger)
	if nil != err {
		logger.Error("Link generation route not registered: link rate limit is not configured", zap.Error(err))
		return
	}
	r.Route("/links", func(r chi.Router) {
		r.With(generateLimit).Post("/generate", linksHandlers.V1GenerateLinkHandler(opts.Links, opts.Authorizer, opts.APIHost, logger))
		if nil == opts.Store || nil == opts.Admins {
			return
		}
		r.Get("/", linksHandlers.V1ListLinks(opts.Store, opts.Admins))
		r.Delete("/{id}", linksHandlers.V1RevokeLink(opts.Store, opts.Admins))
		if nil == opts.Telemetry {
			return
		}
		r.Get("/stats", linksHandlers.V1LinkStats(opts.Store, opts.Telemetry))
		r.Get("/{id}", linksHandlers.V1GetLink(opts.Store, opts.Admins, opts.Telemetry))
	})
}

func registerTrashRoutes(r chi.Router, opts RouterOptions) {
	if nil == opts.Trash {
		return
	}
	r.Get("/trash", handlers.V1ListTrash(opts.Trash))
	r.Delete("/trash", handlers.V1EmptyTrash(opts.Trash))
	r.Post("/trash/{id}/restore", handlers.V1RestoreTrash(opts.Trash))
	r.Delete("/trash/{id}", handlers.V1DeleteTrashItem(opts.Trash))
}

func registerUploadRoutes(r chi.Router, opts RouterOptions) {
	if nil == opts.Uploads {
		return
	}
	r.Post("/uploads", handlers.V1CreateUpload(opts.Uploads))
	r.Put("/uploads/{id}", trackTransfer("upload", handlers.V1PutUploadPart(opts.Uploads)))
	r.Get("/uploads/{id}", handlers.V1GetUpload(opts.Uploads))
	r.Post("/uploads/{id}/complete", handlers.V1CompleteUpload(opts.Uploads))
	r.Delete("/uploads/{id}", handlers.V1AbortUpload(opts.Uploads))
}

func registerErasureRoutes(r chi.Router, opts RouterOptions, logger *zap.Logger) {
	if nil == opts.ErasureStore {
		return
	}
	r.Get("/erasure/files", handlers.V1ListErasureFiles(opts.ErasureStore, opts.Erasure, opts.Authorizer, logger))
	r.Get("/erasure/files/*", handlers.V1GetErasureFile(opts.ErasureStore, opts.Erasure, opts.Authorizer, logger))
	r.Get("/erasure/manifest/*", handlers.HandleErasureManifest(opts.Erasure, opts.Authorizer, logger))
	r.Post("/erasure/verify/*", handlers.V1VerifyErasureFile(opts.Erasure, opts.Authorizer, logger))
	if nil != opts.Jobs {
		r.Post("/erasure/repair/*", handlers.V1RepairErasureFile(opts.ErasureStore, opts.Jobs, opts.Erasure, opts.Authorizer, logger))
	}
}

func registerJobRoutes(r chi.Router, opts RouterOptions) {
	if nil == opts.Jobs || nil == opts.Admins {
		return
	}
	r.Post("/batch", handlers.V1Batch(opts.Engine, opts.Authorizer, opts.Jobs, opts.Trash))
	r.Get("/jobs", handlers.V1ListMyJobs(opts.Jobs))
	r.Get("/jobs/{id}", handlers.V1GetJob(opts.Jobs, opts.Admins))
	r.Post("/jobs/{id}/cancel", handlers.V1CancelJob(opts.Jobs, opts.Admins))
	r.Post("/jobs/{id}/retry-failed", handlers.V1RetryJobFailed(opts.Jobs, opts.Admins))
}

func registerDownloadRoutes(r chi.Router, opts RouterOptions, resources *RouterResources, logger *zap.Logger) {
	downloadLimit, err := limitMiddleware(opts, resources, authMiddleware.RateLimitDownload, opts.ServerConfig.RateLimit.DownloadRPS, opts.ServerConfig.RateLimit.DownloadBurst, logger)
	if nil != err {
		logger.Error("Download routes not registered: download rate limit is not configured", zap.Error(err))
		return
	}
	r.With(downloadLimit).Get("/download/{token}", linksHandlers.V1DownloadLinkConfirmHandler(opts.Links, logger))
	r.With(downloadLimit).Post("/download/{token}", trackTransfer("download", linksHandlers.V1DownloadLinkHandler(opts.Engine, opts.Links, logger)))
}

func writeAckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get(backends.WriteAckHeader)
		if "" == value {
			next.ServeHTTP(w, r)
			return
		}
		ack, err := backends.ParseWriteAck(value)
		if nil != err {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r.WithContext(backends.WithWriteAck(r.Context(), ack)))
	})
}
