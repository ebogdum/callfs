// Package links implements secure, atomic single-use download link functionality.
package links

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metrics"
	"go.uber.org/zap"
)

var (
	ErrLinkInvalid  = errors.New("link is invalid or has been used")
	ErrLinkExpired  = errors.New("link has expired")
	ErrLinkNotFound = errors.New("link not found")
)

// LinkManager manages creation and validation of single-use download links.
type LinkManager struct {
	metadataStore metadata.Store
	secretKey     []byte
	logger        *zap.Logger
}

// NewLinkManager creates a new LinkManager instance.
func NewLinkManager(ms metadata.Store, secretKey string, logger *zap.Logger) (*LinkManager, error) {
	if ms == nil {
		return nil, errors.New("metadata store cannot be nil")
	}
	if secretKey == "" {
		return nil, errors.New("secret key cannot be empty")
	}
	if logger == nil {
		return nil, errors.New("logger cannot be nil")
	}

	// Hash the secret key for HMAC
	h := sha256.Sum256([]byte(secretKey))

	return &LinkManager{
		metadataStore: ms,
		secretKey:     h[:],
		logger:        logger,
	}, nil
}

// GenerateLink creates a new single-use download link for the specified file.
func (lm *LinkManager) GenerateLink(ctx context.Context, filePath string, expiryDuration time.Duration) (string, error) {
	// Generate cryptographically secure random token ID
	tokenIDBytes := make([]byte, 16)
	if _, err := rand.Read(tokenIDBytes); err != nil {
		lm.logger.Error("Failed to generate random token ID", zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	tokenID := base64.URLEncoding.EncodeToString(tokenIDBytes)

	// Compute HMAC-SHA256 signature over tokenID + filePath
	mac := hmac.New(sha256.New, lm.secretKey)
	mac.Write([]byte(tokenID + filePath))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	// Combine token ID and signature
	token := tokenID + "." + signature

	// Create single-use link record
	link := &metadata.SingleUseLink{
		Token:         token,
		FilePath:      filePath,
		Status:        "active",
		ExpiresAt:     time.Now().Add(expiryDuration),
		CreatedAt:     time.Now(),
		HMACSignature: signature,
	}

	// Store in metadata store
	if err := lm.metadataStore.CreateSingleUseLink(ctx, link); err != nil {
		lm.logger.Error("Failed to store single-use link",
			zap.String("token", TruncateToken(token)),
			zap.String("file_path", filePath),
			zap.Error(err))
		return "", fmt.Errorf("failed to store link: %w", err)
	}

	lm.logger.Info("Generated single-use download link",
		zap.String("token", TruncateToken(token)),
		zap.String("file_path", filePath),
		zap.Time("expires_at", link.ExpiresAt))

	// Record metrics
	metrics.SingleUseLinkGenerationsTotal.Inc()

	return token, nil
}

// ValidateAndInvalidateLink validates a download link and atomically marks it as used.
// Returns the file path if valid, or an error if invalid/expired/already used.
func (lm *LinkManager) ValidateAndInvalidateLink(ctx context.Context, token, userIP string) (string, error) {
	// Retrieve link from metadata store
	link, err := lm.metadataStore.GetSingleUseLink(ctx, token)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			lm.logger.Warn("Single-use link not found", zap.String("token", TruncateToken(token)))
			metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("not_found").Inc()
			return "", ErrLinkNotFound
		}
		lm.logger.Error("Failed to retrieve single-use link",
			zap.String("token", TruncateToken(token)),
			zap.Error(err))
		return "", fmt.Errorf("failed to retrieve link: %w", err)
	}

	// Check if link has expired
	if time.Now().After(link.ExpiresAt) {
		lm.logger.Warn("Single-use link has expired",
			zap.String("token", TruncateToken(token)),
			zap.Time("expired_at", link.ExpiresAt))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("expired").Inc()
		return "", ErrLinkExpired
	}

	// Check if link is still active
	if link.Status != "active" {
		lm.logger.Warn("Single-use link is not active",
			zap.String("token", TruncateToken(token)),
			zap.String("status", link.Status))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("invalid").Inc()
		return "", ErrLinkInvalid
	}

	// Verify HMAC signature
	if !lm.verifySignature(token, link.FilePath) {
		lm.logger.Warn("Single-use link signature verification failed",
			zap.String("token", TruncateToken(token)),
			zap.String("file_path", link.FilePath))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("invalid").Inc()
		return "", ErrLinkInvalid
	}

	// Atomically mark link as used
	now := time.Now()
	if err := lm.metadataStore.UpdateSingleUseLink(ctx, token, "used", &now, &userIP); err != nil {
		lm.logger.Error("Failed to mark single-use link as used",
			zap.String("token", TruncateToken(token)),
			zap.String("user_ip", userIP),
			zap.Error(err))
		return "", fmt.Errorf("failed to invalidate link: %w", err)
	}

	lm.logger.Info("Single-use link consumed",
		zap.String("token", TruncateToken(token)),
		zap.String("file_path", link.FilePath),
		zap.String("user_ip", userIP),
		zap.Time("used_at", now))

	// Record successful consumption
	metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("success").Inc()

	return link.FilePath, nil
}

// TruncateToken returns a redacted token suitable for logs.
func TruncateToken(token string) string {
	if len(token) <= 8 {
		return token
	}

	return token[:8] + "..."
}

// verifySignature verifies the HMAC signature in the token.
func (lm *LinkManager) verifySignature(token, filePath string) bool {
	// Split token into ID and signature
	parts := []byte(token)
	dotIndex := -1
	for i, b := range parts {
		if b == '.' {
			dotIndex = i
			break
		}
	}
	if dotIndex == -1 {
		return false
	}

	tokenID := string(parts[:dotIndex])
	providedSignature := string(parts[dotIndex+1:])

	// Compute expected signature
	mac := hmac.New(sha256.New, lm.secretKey)
	mac.Write([]byte(tokenID + filePath))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	// Use constant-time comparison
	return hmac.Equal([]byte(providedSignature), []byte(expectedSignature))
}
