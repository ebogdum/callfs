package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/metrics"
	"github.com/ebogdum/callfs/server/handlers"
	linksHandlers "github.com/ebogdum/callfs/server/handlers/links"
	authMiddleware "github.com/ebogdum/callfs/server/middleware"
)

// NewRouter creates and configures the HTTP router
func NewRouter(
	engine *core.Engine,
	authenticator auth.Authenticator,
	authorizer auth.Authorizer,
	linkManager *links.LinkManager,
	serverConfig *config.ServerConfig,
	backendConfig *config.BackendConfig,
	apiHost string,
	logger *zap.Logger,
) chi.Router {
	// Initialize metrics
	metrics.RegisterMetrics()

	r := chi.NewRouter()

	// Basic middleware
	r.Use(authMiddleware.V1RequestIDMiddleware())
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(authMiddleware.V1SecurityHeaders())

	// Custom logging and metrics middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			duration := time.Since(start)

			// Record metrics
			metrics.HTTPRequestsTotal.WithLabelValues(
				r.Method,
				r.URL.Path,
				http.StatusText(ww.Status()),
			).Inc()

			metrics.HTTPRequestDuration.WithLabelValues(
				r.Method,
				r.URL.Path,
			).Observe(duration.Seconds())

			logger.Info("HTTP request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", duration),
				zap.String("user_agent", r.UserAgent()),
				zap.String("remote_addr", r.RemoteAddr))
		})
	})

	// Health check endpoint (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			// Log error but don't change response since headers are already written
			slog.Error("Failed to write health check response", "error", err)
		}
	})

	// Metrics endpoint (no auth required)
	r.Handle("/metrics", promhttp.Handler())

	// API v1 routes with authentication
	r.Route("/v1", func(r chi.Router) {
		// Apply authentication middleware to all API routes
		r.Use(authMiddleware.V1AuthMiddleware(authenticator, logger))

		// File operations
		r.Route("/files", func(r chi.Router) {
			// Handle all paths with /*
			r.Get("/*", handlers.V1GetFile(engine, authorizer, serverConfig, logger))
			r.Head("/*", handlers.V1HeadFileEnhanced(engine, authorizer, logger))
			r.Post("/*", handlers.V1PostFileEnhanced(engine, authorizer, backendConfig, serverConfig, logger))
			r.Put("/*", handlers.V1PutFileEnhanced(engine, authorizer, backendConfig, serverConfig, logger))
			r.Delete("/*", handlers.V1DeleteFileEnhanced(engine, authorizer, logger))
		})

		// Directory listing API (moved from /api/directories to /directories)
		r.Route("/directories", func(r chi.Router) {
			r.Get("/*", handlers.V1ListDirectory(engine, authorizer, logger))
		})

		// Single-use link operations
		r.Route("/links", func(r chi.Router) {
			// Apply rate limiting specifically to link generation (100 requests per second, burst of 1)
			linkRateLimiter := rate.NewLimiter(100, 1)
			r.With(authMiddleware.V1RateLimitMiddleware(linkRateLimiter, logger)).
				Post("/generate", linksHandlers.V1GenerateLinkHandler(linkManager, apiHost, logger))
		})
	})

	// Single-use download endpoint (no auth required)
	r.Get("/download/{token}", linksHandlers.V1DownloadLinkHandler(engine, linkManager, logger))

	logger.Info("HTTP router configured successfully")

	return r
}
