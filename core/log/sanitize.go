// Package log provides secure logging utilities with data sanitization capabilities.
package log

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

// SanitizationMode controls how sensitive data is handled in logs
type SanitizationMode int

const (
	// ProductionMode hashes sensitive data for production use
	ProductionMode SanitizationMode = iota
	// DevelopmentMode shows truncated sensitive data for debugging
	DevelopmentMode
	// DebugMode shows full sensitive data (only for development)
	DebugMode
)

var currentMode = ProductionMode

func init() {
	// Check environment variable to set mode
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

// SanitizePath sanitizes file paths for logging based on the current mode
func SanitizePath(path string) string {
	if path == "" {
		return ""
	}

	switch currentMode {
	case ProductionMode:
		// Hash the path to prevent leaking sensitive filenames
		hash := sha256.Sum256([]byte(path))
		return fmt.Sprintf("hash:%x", hash[:8]) // Show first 8 bytes of hash
	case DevelopmentMode:
		// Show truncated path for debugging
		if len(path) <= 20 {
			return path
		}
		return path[:10] + "..." + path[len(path)-7:]
	case DebugMode:
		// Show full path (only for development)
		return path
	default:
		return SanitizePath(path) // Default to production mode
	}
}

// SanitizeUserID sanitizes user IDs for logging
func SanitizeUserID(userID string) string {
	if userID == "" {
		return ""
	}

	switch currentMode {
	case ProductionMode:
		// Hash the user ID
		hash := sha256.Sum256([]byte(userID))
		return fmt.Sprintf("user_hash:%x", hash[:6]) // Show first 6 bytes of hash
	case DevelopmentMode:
		// Show truncated user ID
		if len(userID) <= 8 {
			return userID
		}
		return userID[:4] + "****"
	case DebugMode:
		// Show full user ID
		return userID
	default:
		return SanitizeUserID(userID)
	}
}

// SanitizeBackendInfo sanitizes backend type information
func SanitizeBackendInfo(backendType string) string {
	// Backend type is generally safe to log as it's not user data
	// But we can still sanitize for consistency
	switch currentMode {
	case ProductionMode, DevelopmentMode, DebugMode:
		return backendType
	default:
		return backendType
	}
}

// SanitizeSize sanitizes file size information (generally safe to log)
func SanitizeSize(size int64) int64 {
	// File sizes are generally not sensitive, but could be rounded in production
	switch currentMode {
	case ProductionMode:
		// Round to nearest KB to obscure exact sizes
		return (size + 512) / 1024 * 1024
	case DevelopmentMode, DebugMode:
		return size
	default:
		return size
	}
}

// LogFields provides a structured way to handle sensitive logging fields
type LogFields struct {
	Path      string
	UserID    string
	Backend   string
	Size      int64
	Operation string
}

// Sanitize returns sanitized versions of all fields
func (lf LogFields) Sanitize() LogFields {
	return LogFields{
		Path:      SanitizePath(lf.Path),
		UserID:    SanitizeUserID(lf.UserID),
		Backend:   SanitizeBackendInfo(lf.Backend),
		Size:      SanitizeSize(lf.Size),
		Operation: lf.Operation, // Operations are generally safe
	}
}
