package log

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

type SanitizationMode int

const (
	ProductionMode SanitizationMode = iota
	DevelopmentMode
	DebugMode
)

var currentMode = ProductionMode

var hashKey, hashKeyErr = newHashKey()

func init() {
	if mode := os.Getenv("CALLFS_LOG_MODE"); mode != "" {
		switch strings.ToLower(mode) {
		case "production":
			currentMode = ProductionMode
		case "development":
			currentMode = DevelopmentMode
		case "debug":
			currentMode = DebugMode
		}
	}
}

func newHashKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); nil != err {
		return nil, err
	}
	return key, nil
}

func keyedHashPrefix(value string, size int) string {
	if nil != hashKeyErr {
		return "unavailable"
	}
	mac := hmac.New(sha256.New, hashKey)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)[:size])
}

func SanitizePath(path string) string {
	if path == "" {
		return ""
	}

	switch currentMode {
	case ProductionMode:
		return "hash:" + keyedHashPrefix(path, 8)
	case DevelopmentMode:
		if len(path) <= 20 {
			return path
		}
		return path[:10] + "..." + path[len(path)-7:]
	case DebugMode:
		return path
	default:
		return "hash:" + keyedHashPrefix(path, 8)
	}
}

func SanitizeUserID(userID string) string {
	if userID == "" {
		return ""
	}

	switch currentMode {
	case ProductionMode:
		return "user_hash:" + keyedHashPrefix(userID, 6)
	case DevelopmentMode:
		if len(userID) <= 8 {
			return userID
		}
		return userID[:4] + "****"
	case DebugMode:
		return userID
	default:
		return "user_hash:" + keyedHashPrefix(userID, 6)
	}
}

func SanitizeSize(size int64) int64 {
	switch currentMode {
	case ProductionMode:
		return ((size + 512) / 1024) * 1024
	default:
		return size
	}
}

type LogFields struct {
	Path      string
	UserID    string
	Backend   string
	Size      int64
	Operation string
}

func (lf LogFields) Sanitize() LogFields {
	return LogFields{
		Path:      SanitizePath(lf.Path),
		UserID:    SanitizeUserID(lf.UserID),
		Backend:   lf.Backend,
		Size:      SanitizeSize(lf.Size),
		Operation: lf.Operation,
	}
}
