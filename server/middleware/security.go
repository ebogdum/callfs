package middleware

import (
	"net/http"
)

// V1SecurityHeaders adds common security headers to HTTP responses
func V1SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Content Security Policy - removed 'unsafe-inline' for better security
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; form-action 'self'")

			// Strict Transport Security (HSTS)
			if r.TLS != nil {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			// X-Content-Type-Options
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// X-Frame-Options
			w.Header().Set("X-Frame-Options", "DENY")

			// Referrer Policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Permissions Policy
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()")

			// Cross-Origin-Opener-Policy
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

			// Cross-Origin-Embedder-Policy
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")

			next.ServeHTTP(w, r)
		})
	}
}
