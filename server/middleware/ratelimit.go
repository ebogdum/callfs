package middleware

import (
	"net/http"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// V1RateLimitMiddleware creates a middleware that applies rate limiting to HTTP requests.
// It uses a simple fixed-rate limiter for all requests. For production use, consider
// implementing per-client rate limiting using a more sophisticated approach.
func V1RateLimitMiddleware(limiter *rate.Limiter, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the request is allowed by the rate limiter
			if !limiter.Allow() {
				logger.Warn("Request rate limited",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
					zap.String("user_agent", r.UserAgent()))

				// Send rate limit error response directly
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if _, err := w.Write([]byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}`)); err != nil {
					logger.Error("Failed to write rate limit error response", zap.Error(err))
				}
				return
			}

			// If rate limit check passes, continue to the next handler
			next.ServeHTTP(w, r)
		})
	}
}
