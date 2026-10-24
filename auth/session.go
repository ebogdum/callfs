package auth

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const sessionKeyInfo = "callfs-ui-session"

var (
	ErrInvalidSession    = errors.New("invalid session")
	ErrSessionExpired    = errors.New("session expired")
	ErrSessionKeyMissing = errors.New("session key is empty")
)

type SessionClaims struct {
	UserID      string
	Fingerprint string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	CSRF        string
}

type sessionPayload struct {
	UserID      string `json:"uid"`
	Fingerprint string `json:"fp"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
	CSRF        string `json:"csrf"`
}

type SessionManager struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func DeriveSessionKey(internalProxySecret string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, []byte(internalProxySecret), nil, sessionKeyInfo, sha256.Size)
	if nil != err {
		return nil, fmt.Errorf("failed to derive session key: %w", err)
	}
	return key, nil
}

func NewSessionManager(key []byte, ttl time.Duration) *SessionManager {
	return &SessionManager{key: key, ttl: ttl, now: time.Now}
}

func (m *SessionManager) Issue(userID, keyFingerprint string) (string, string, time.Time, error) {
	if 0 == len(m.key) {
		return "", "", time.Time{}, ErrSessionKeyMissing
	}
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); nil != err {
		return "", "", time.Time{}, fmt.Errorf("failed to generate csrf token: %w", err)
	}
	csrf := base64.RawURLEncoding.EncodeToString(csrfBytes)
	issuedAt := m.now().Unix()
	expiresAt := issuedAt + int64(m.ttl/time.Second)
	payload, err := json.Marshal(sessionPayload{
		UserID:      userID,
		Fingerprint: keyFingerprint,
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
		CSRF:        csrf,
	})
	if nil != err {
		return "", "", time.Time{}, fmt.Errorf("failed to encode session: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + m.sign(encoded), csrf, time.Unix(expiresAt, 0).UTC(), nil
}

func (m *SessionManager) Verify(cookieValue string) (SessionClaims, error) {
	if 0 == len(m.key) {
		return SessionClaims{}, ErrSessionKeyMissing
	}
	encoded, signature, found := strings.Cut(cookieValue, ".")
	if !found || "" == encoded {
		return SessionClaims{}, ErrInvalidSession
	}
	if !hmac.Equal([]byte(signature), []byte(m.sign(encoded))) {
		return SessionClaims{}, ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if nil != err {
		return SessionClaims{}, ErrInvalidSession
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); nil != err {
		return SessionClaims{}, ErrInvalidSession
	}
	claims := SessionClaims{
		UserID:      payload.UserID,
		Fingerprint: payload.Fingerprint,
		IssuedAt:    time.Unix(payload.IssuedAt, 0).UTC(),
		ExpiresAt:   time.Unix(payload.ExpiresAt, 0).UTC(),
		CSRF:        payload.CSRF,
	}
	if !m.now().Before(claims.ExpiresAt) {
		return claims, ErrSessionExpired
	}
	return claims, nil
}

func (m *SessionManager) sign(encoded string) string {
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(encoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
