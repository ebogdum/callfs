package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/metrics"
	"github.com/ebogdum/callfs/v2/telemetry"
)

type contextKey string

const (
	requestScopeKey contextKey = "request_scope"
)

const (
	sessionCookieName   = "callfs_session"
	csrfHeaderName      = "X-CallFS-CSRF"
	onBehalfHeaderName  = "X-CallFS-On-Behalf-Of"
	internalProxyUserID = "internal-proxy"
	maxDelegatedUserLen = 256
)

type requestScope struct {
	requestID     string
	userID        string
	authenticated bool
	session       *auth.SessionClaims
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type sessionUserResolver interface {
	UserForFingerprint(fingerprint string) (string, bool)
}

var (
	authenticationFailedBody  = mustMarshalErrorBody("AUTHENTICATION_FAILED", auth.ErrAuthenticationFailed)
	permissionDeniedBody      = mustMarshalErrorBody("PERMISSION_DENIED", auth.ErrPermissionDenied)
	errInternalAuthentication = errors.New("internal authentication error")
	errCSRFFailed             = errors.New("csrf token missing or invalid")
	authRateLimitedBody       = []byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}`)
	authFailureRate           = rate.Every(6 * time.Second)
	authFailureBurst          = 10
)

func V1AuthMiddleware(authenticator auth.Authenticator, sessions *auth.SessionManager, fingerprints sessionUserResolver, logger *zap.Logger, onAuthFailure func(reason string, r *http.Request)) func(http.Handler) http.Handler {
	failures := newPerIPRateLimiter(authFailureRate, authFailureBurst)
	reportFailure := func(reason string, r *http.Request) {
		metrics.AuthFailuresTotal.WithLabelValues(reason).Inc()
		if nil != onAuthFailure {
			onAuthFailure(reason, r)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authHeader := r.Header.Get("Authorization"); "" != authHeader {
				clientFailures := failures.getLimiter(remoteIP(r))
				if 1 > clientFailures.Tokens() {
					metrics.AuthFailuresTotal.WithLabelValues("blocked").Inc()
					logger.Warn("Authentication blocked after repeated failures",
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.String("remote_addr", r.RemoteAddr))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					if _, err := w.Write(authRateLimitedBody); err != nil {
						logger.Error("Failed to write rate limit error response", zap.Error(err))
					}
					return
				}
				userID, err := authenticator.Authenticate(r.Context(), authHeader)
				if nil != err {
					if errors.Is(err, auth.ErrAuthenticationFailed) {
						logger.Debug("Authentication failed", zap.Error(err))
						clientFailures.Allow()
						reportFailure("wrong_key", r)
						sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
						return
					}
					logger.Error("Authentication error", zap.Error(err))
					sendErrorResponse(w, logger, errInternalAuthentication, http.StatusInternalServerError)
					return
				}

				if delegated := r.Header.Get(onBehalfHeaderName); "" != delegated {
					if internalProxyUserID != userID || !validDelegatedUser(delegated) {
						metrics.AuthFailuresTotal.WithLabelValues("bad_delegation").Inc()
						sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
						return
					}
					userID = delegated
				}

				logger.Debug("User authenticated", zap.String("user_id", userID))

				next.ServeHTTP(w, withAuthenticatedScope(r, userID, nil))
				return
			}

			cookie, cookieErr := r.Cookie(sessionCookieName)
			if nil == sessions || nil == fingerprints || nil != cookieErr {
				logger.Debug("Missing Authorization header and session cookie")
				reportFailure("missing_key", r)
				sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
				return
			}

			claims, err := sessions.Verify(cookie.Value)
			if nil != err {
				if errors.Is(err, auth.ErrSessionKeyMissing) {
					logger.Error("Session verification error", zap.Error(err))
					sendErrorResponse(w, logger, errInternalAuthentication, http.StatusInternalServerError)
					return
				}
				if errors.Is(err, auth.ErrSessionExpired) {
					reportFailure("expired_session", r)
				} else {
					reportFailure("wrong_key", r)
				}
				sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
				return
			}

			userID, known := fingerprints.UserForFingerprint(claims.Fingerprint)
			if !known || claims.UserID != userID {
				reportFailure("wrong_key", r)
				sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
				return
			}

			if !isSafeMethod(r.Method) && !csrfTokenMatches(r, claims.CSRF) {
				logger.Debug("CSRF check failed", zap.String("method", r.Method), zap.String("path", r.URL.Path))
				reportFailure("csrf", r)
				sendErrorResponse(w, logger, errCSRFFailed, http.StatusForbidden)
				return
			}

			logger.Debug("Session authenticated", zap.String("user_id", userID))

			next.ServeHTTP(w, withAuthenticatedScope(r, userID, &claims))
		})
	}
}

func withAuthenticatedScope(r *http.Request, userID string, session *auth.SessionClaims) *http.Request {
	telemetry.SetAuditUser(r.Context(), userID)
	if scope, ok := r.Context().Value(requestScopeKey).(*requestScope); ok {
		scope.userID = userID
		scope.authenticated = true
		scope.session = session
		return r
	}
	ctx := context.WithValue(r.Context(), requestScopeKey, &requestScope{userID: userID, authenticated: true, session: session})
	return r.WithContext(ctx)
}

func isSafeMethod(method string) bool {
	return http.MethodGet == method || http.MethodHead == method || http.MethodOptions == method
}

func csrfTokenMatches(r *http.Request, expected string) bool {
	presented := r.Header.Get(csrfHeaderName)
	if "" == presented || "" == expected {
		return false
	}
	return 1 == subtle.ConstantTimeCompare([]byte(presented), []byte(expected))
}

func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if nil != err {
		return r.RemoteAddr
	}
	return ip
}

func V1RequestIDMiddleware() func(http.Handler) http.Handler {
	generator := newRequestIDGenerator()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope := &requestScope{requestID: generator.next()}

			w.Header().Set("X-Request-ID", scope.requestID)

			ctx := context.WithValue(r.Context(), requestScopeKey, scope)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

type requestIDGenerator struct {
	prefix  uint32
	counter atomic.Uint32
}

func newRequestIDGenerator() *requestIDGenerator {
	var seed [4]byte
	prefix := uint32(time.Now().UnixNano())
	if _, err := rand.Read(seed[:]); err == nil {
		prefix = binary.BigEndian.Uint32(seed[:])
	}
	return &requestIDGenerator{prefix: prefix}
}

func (g *requestIDGenerator) next() string {
	var b [8]byte
	binary.BigEndian.PutUint32(b[0:4], g.prefix)
	binary.BigEndian.PutUint32(b[4:8], g.counter.Add(1))
	return hex.EncodeToString(b[:])
}

func GetUserID(ctx context.Context) (string, bool) {
	scope, ok := ctx.Value(requestScopeKey).(*requestScope)
	if !ok || !scope.authenticated {
		return "", false
	}
	return scope.userID, true
}

func GetSession(ctx context.Context) (auth.SessionClaims, bool) {
	scope, ok := ctx.Value(requestScopeKey).(*requestScope)
	if !ok || !scope.authenticated || nil == scope.session {
		return auth.SessionClaims{}, false
	}
	return *scope.session, true
}

func errorCodeFor(err error) string {
	switch {
	case errors.Is(err, auth.ErrAuthenticationFailed):
		return "AUTHENTICATION_FAILED"
	case errors.Is(err, auth.ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, errCSRFFailed):
		return "CSRF_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}

func mustMarshalErrorBody(code string, err error) []byte {
	body, marshalErr := json.Marshal(errorBody{Code: code, Message: err.Error()})
	if marshalErr != nil {
		panic(marshalErr)
	}
	return body
}

func errorBodyFor(err error, errorCode string) ([]byte, error) {
	switch err {
	case auth.ErrAuthenticationFailed:
		return authenticationFailedBody, nil
	case auth.ErrPermissionDenied:
		return permissionDeniedBody, nil
	}
	return json.Marshal(errorBody{Code: errorCode, Message: err.Error()})
}

func sendErrorResponse(w http.ResponseWriter, logger *zap.Logger, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	if http.StatusUnauthorized == statusCode {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(statusCode)

	errorCode := errorCodeFor(err)
	body, marshalErr := errorBodyFor(err, errorCode)
	if marshalErr != nil {
		logger.Error("Failed to marshal error response", zap.Error(marshalErr))
		return
	}
	if _, writeErr := w.Write(body); writeErr != nil {
		logger.Error("Failed to write error response", zap.Error(writeErr))
	}

	logger.Debug("Error response sent",
		zap.String("error_code", errorCode),
		zap.Int("status_code", statusCode),
		zap.Error(err))
}

func validDelegatedUser(id string) bool {
	if maxDelegatedUserLen < len(id) || internalProxyUserID == id {
		return false
	}
	for _, r := range id {
		if ' ' >= r || 0x7f == r {
			return false
		}
	}
	return true
}
