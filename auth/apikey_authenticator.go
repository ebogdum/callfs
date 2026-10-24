package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
)

const (
	SourceAPIKeyUsers = "api_key_users"
	SourceAPIKeys     = "api_keys"
	SourceReserved    = "reserved"
)

type apiKeyCredential struct {
	digest       [sha256.Size]byte
	fingerprint  string
	keyHint      string
	userID       string
	legacyUserID string
	source       string
	index        int
}

type UserInfo struct {
	UserID         string `json:"user_id"`
	Source         string `json:"source"`
	Index          *int   `json:"index"`
	KeyHint        string `json:"key_hint"`
	KeyFingerprint string `json:"key_fingerprint"`
}

type APIKeyAuthenticator struct {
	mu           sync.RWMutex
	credentials  []apiKeyCredential
	userByDigest map[[sha256.Size]byte]string
}

func NewAPIKeyAuthenticator(keys []string, keyUsers map[string]string, internalProxySecret string) *APIKeyAuthenticator {
	credentials := buildCredentials(keys, keyUsers, internalProxySecret)
	return &APIKeyAuthenticator{
		credentials:  credentials,
		userByDigest: indexByDigest(credentials),
	}
}

func (a *APIKeyAuthenticator) Reload(apiKeys []string, apiKeyUsers map[string]string, internalSecret string) {
	credentials := buildCredentials(apiKeys, apiKeyUsers, internalSecret)
	userByDigest := indexByDigest(credentials)
	a.mu.Lock()
	a.credentials = credentials
	a.userByDigest = userByDigest
	a.mu.Unlock()
}

func indexByDigest(credentials []apiKeyCredential) map[[sha256.Size]byte]string {
	userByDigest := make(map[[sha256.Size]byte]string, len(credentials))
	for i := range credentials {
		userByDigest[credentials[i].digest] = credentials[i].userID
	}
	return userByDigest
}

func buildCredentials(keys []string, keyUsers map[string]string, internalProxySecret string) []apiKeyCredential {
	validKeys := make(map[string]apiKeyCredential, len(keys)+len(keyUsers)+1)
	positional := 0
	for _, key := range keys {
		if "" != key {
			validKeys[key] = apiKeyCredential{
				userID:       positionalUserID(key),
				legacyUserID: "api-user-" + strconv.Itoa(positional),
				source:       SourceAPIKeys,
				index:        positional,
			}
			positional++
		}
	}
	for userID, key := range keyUsers {
		if key != "" && userID != "" {
			validKeys[key] = apiKeyCredential{userID: userID, source: SourceAPIKeyUsers, index: -1}
		}
	}
	if internalProxySecret != "" {
		validKeys[internalProxySecret] = apiKeyCredential{userID: "internal-proxy", source: SourceReserved, index: -1}
	}

	credentials := make([]apiKeyCredential, 0, len(validKeys))
	for key, credential := range validKeys {
		credential.digest = sha256.Sum256([]byte(key))
		if SourceReserved != credential.source {
			credential.fingerprint = Fingerprint(key)
			credential.keyHint = maskedKeyHint(key)
		}
		credentials = append(credentials, credential)
	}
	return credentials
}

func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (string, error) {
	if 7 <= len(token) && strings.EqualFold("Bearer ", token[:7]) {
		token = token[7:]
	}
	token = strings.TrimSpace(token)

	if token == "" {
		return "", ErrAuthenticationFailed
	}

	tokenDigest := sha256.Sum256([]byte(token))
	a.mu.RLock()
	foundUser, found := a.userByDigest[tokenDigest]
	a.mu.RUnlock()
	if false == found {
		return "", ErrAuthenticationFailed
	}
	return foundUser, nil
}

func positionalUserID(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "api-user-" + hex.EncodeToString(digest[:16])
}

func Fingerprint(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:8])
}

func maskedKeyHint(key string) string {
	if len(key) < 8 {
		return ""
	}
	return key[len(key)-4:]
}

func (a *APIKeyAuthenticator) UserForFingerprint(fingerprint string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := range a.credentials {
		credential := &a.credentials[i]
		if "" != fingerprint && fingerprint == credential.fingerprint {
			return credential.userID, true
		}
	}
	return "", false
}

func (a *APIKeyAuthenticator) Users() []UserInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	users := make([]UserInfo, 0, len(a.credentials))
	for i := range a.credentials {
		credential := &a.credentials[i]
		info := UserInfo{
			UserID:         credential.userID,
			Source:         credential.source,
			KeyHint:        credential.keyHint,
			KeyFingerprint: credential.fingerprint,
		}
		if SourceAPIKeys == credential.source {
			index := credential.index
			info.Index = &index
		}
		users = append(users, info)
	}
	return users
}

func (a *APIKeyAuthenticator) LegacyAliases(userID string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := range a.credentials {
		credential := &a.credentials[i]
		if userID == credential.userID && "" != credential.legacyUserID {
			return []string{credential.legacyUserID}
		}
	}
	return nil
}
