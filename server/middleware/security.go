package middleware

import (
	"net/http"
)

var (
	securityHeadersAlways = buildSecurityHeaders()
	hstsHeaderValue       = []string{"max-age=31536000; includeSubDomains; preload"}
)

func buildSecurityHeaders() map[string][]string {
	return map[string][]string{
		"Content-Security-Policy":      {"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; form-action 'self'"},
		"X-Content-Type-Options":       {"nosniff"},
		"X-Frame-Options":              {"DENY"},
		"Referrer-Policy":              {"strict-origin-when-cross-origin"},
		"Permissions-Policy":           {"geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()"},
		"Cross-Origin-Opener-Policy":   {"same-origin"},
		"Cross-Origin-Embedder-Policy": {"require-corp"},
	}
}

func V1SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			for key, value := range securityHeadersAlways {
				header[key] = value
			}
			if nil != r.TLS {
				header["Strict-Transport-Security"] = hstsHeaderValue
			}
			next.ServeHTTP(w, r)
		})
	}
}
