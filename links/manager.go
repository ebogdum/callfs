package links

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metrics"
	"go.uber.org/zap"
)

var (
	ErrLinkInvalid        = errors.New("link is invalid or has been used")
	ErrLinkExpired        = errors.New("link has expired")
	ErrLinkNotFound       = errors.New("link not found")
	ErrTooManyActiveLinks = errors.New("too many active download links")
)

const maxActiveLinksPerOwner = 1000

type LinkManager struct {
	metadataStore metadata.Store
	secretKey     []byte
	logger        *zap.Logger
}

func NewLinkManager(ms metadata.Store, secretKey string, logger *zap.Logger) (*LinkManager, error) {
	if nil == ms {
		return nil, errors.New("metadata store cannot be nil")
	}
	if "" == secretKey {
		return nil, errors.New("secret key cannot be empty")
	}
	if nil == logger {
		return nil, errors.New("logger cannot be nil")
	}

	h := sha256.Sum256([]byte(secretKey))

	return &LinkManager{
		metadataStore: ms,
		secretKey:     h[:],
		logger:        logger,
	}, nil
}

func (lm *LinkManager) GenerateOwnedLink(ctx context.Context, owner, filePath string, expiryDuration time.Duration) (*metadata.SingleUseLink, error) {
	active, err := lm.metadataStore.CountActiveSingleUseLinks(ctx, owner, time.Now())
	if nil != err {
		return nil, fmt.Errorf("failed to count active links: %w", err)
	}
	if maxActiveLinksPerOwner <= active {
		return nil, ErrTooManyActiveLinks
	}
	return lm.createLink(ctx, owner, filePath, expiryDuration)
}

func (lm *LinkManager) GenerateLink(ctx context.Context, filePath string, expiryDuration time.Duration) (string, error) {
	link, err := lm.createLink(ctx, "", filePath, expiryDuration)
	if nil != err {
		return "", err
	}
	return link.Token, nil
}

func (lm *LinkManager) createLink(ctx context.Context, owner, filePath string, expiryDuration time.Duration) (*metadata.SingleUseLink, error) {
	tokenIDBytes := make([]byte, 16)
	if _, err := rand.Read(tokenIDBytes); nil != err {
		lm.logger.Error("Failed to generate random token ID", zap.Error(err))
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	tokenID := base64.URLEncoding.EncodeToString(tokenIDBytes)

	mac := hmac.New(sha256.New, lm.secretKey)
	mac.Write([]byte(tokenID))
	mac.Write([]byte(filePath))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	token := tokenID + "." + signature

	link := &metadata.SingleUseLink{
		Token:         token,
		FilePath:      filePath,
		Owner:         owner,
		Status:        "active",
		ExpiresAt:     time.Now().Add(expiryDuration),
		CreatedAt:     time.Now(),
		HMACSignature: signature,
	}

	if err := lm.metadataStore.CreateSingleUseLink(ctx, link); nil != err {
		lm.logger.Error("Failed to store single-use link",
			zap.String("token", TruncateToken(token)),
			zap.String("file_path", filePath),
			zap.Error(err))
		return nil, fmt.Errorf("failed to store link: %w", err)
	}

	lm.logger.Info("Generated single-use download link",
		zap.String("token", TokenPrefix(token)),
		zap.String("file_path", filePath),
		zap.Time("expires_at", link.ExpiresAt))

	metrics.SingleUseLinkGenerationsTotal.Inc()
	metrics.SingleUseLinksActive.Inc()

	return link, nil
}

func (lm *LinkManager) ValidateAndInvalidateLink(ctx context.Context, token, userIP string) (string, error) {
	if !isWellFormedToken(token) {
		lm.logger.Warn("Single-use link not found", zap.String("token", TruncateToken(token)))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("not_found").Inc()
		return "", ErrLinkNotFound
	}

	return lm.consumeLink(ctx, token, userIP)
}

func (lm *LinkManager) InspectLink(ctx context.Context, token string) (string, error) {
	if !isWellFormedToken(token) {
		return "", ErrLinkNotFound
	}

	link, err := lm.metadataStore.GetSingleUseLink(ctx, token)
	if nil != err {
		if errors.Is(err, metadata.ErrNotFound) {
			return "", ErrLinkNotFound
		}
		return "", fmt.Errorf("failed to retrieve link: %w", err)
	}

	if "active" != link.Status {
		return "", ErrLinkInvalid
	}
	if time.Now().After(link.ExpiresAt) {
		return "", ErrLinkExpired
	}
	if !lm.verifySignature(token, link.FilePath) {
		return "", ErrLinkInvalid
	}

	return link.FilePath, nil
}

func TruncateToken(token string) string {
	digest := sha256.Sum256([]byte(token))

	return hex.EncodeToString(digest[:8])
}

func TokenPrefix(token string) string {
	tokenID, _, _ := strings.Cut(token, ".")
	if 8 < len(tokenID) {
		return tokenID[:8]
	}
	return tokenID
}

func (lm *LinkManager) verifySignature(token, filePath string) bool {
	tokenID, providedSignature, found := strings.Cut(token, ".")
	if !found {
		return false
	}

	mac := hmac.New(sha256.New, lm.secretKey)
	mac.Write([]byte(tokenID))
	mac.Write([]byte(filePath))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(providedSignature), []byte(expectedSignature))
}

var encodedTokenLen = base64.URLEncoding.EncodedLen(16) + 1 + base64.URLEncoding.EncodedLen(sha256.Size)

func isWellFormedToken(token string) bool {
	return len(token) == encodedTokenLen && strings.IndexByte(token, '.') == base64.URLEncoding.EncodedLen(16)
}

func (lm *LinkManager) consumeLink(ctx context.Context, token, userIP string) (string, error) {
	now := time.Now()
	link, consumed, err := lm.metadataStore.ConsumeSingleUseLink(ctx, token, userIP, now)
	if nil != err {
		if errors.Is(err, metadata.ErrNotFound) {
			lm.logger.Warn("Single-use link not found", zap.String("token", TruncateToken(token)))
			metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("not_found").Inc()
			return "", ErrLinkNotFound
		}
		lm.logger.Error("Failed to consume single-use link",
			zap.String("token", TruncateToken(token)),
			zap.Error(err))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to retrieve link: %w", err)
	}

	if !consumed {
		if now.After(link.ExpiresAt) {
			lm.logger.Warn("Single-use link has expired",
				zap.String("token", TruncateToken(token)),
				zap.Time("expired_at", link.ExpiresAt))
			metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("expired").Inc()
			return "", ErrLinkExpired
		}
		lm.logger.Warn("Single-use link is not active",
			zap.String("token", TruncateToken(token)),
			zap.String("status", link.Status))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("invalid").Inc()
		return "", ErrLinkInvalid
	}
	metrics.SingleUseLinksActive.Dec()

	if !lm.verifySignature(token, link.FilePath) {
		lm.logger.Warn("Single-use link signature verification failed",
			zap.String("token", TruncateToken(token)),
			zap.String("file_path", link.FilePath))
		metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("invalid").Inc()
		return "", ErrLinkInvalid
	}

	lm.logger.Info("Single-use link consumed",
		zap.String("token", TruncateToken(token)),
		zap.String("file_path", link.FilePath),
		zap.String("user_ip", userIP),
		zap.Time("used_at", now))
	metrics.SingleUseLinkConsumptionsTotal.WithLabelValues("success").Inc()

	return link.FilePath, nil
}

const activeLinkPageSize = 1000

func RefreshActiveLinks(ctx context.Context, store metadata.Store, now time.Time) (int, error) {
	filter := metadata.LinkFilter{Status: "active", Now: now, Limit: activeLinkPageSize}
	active := 0
	for {
		page, next, err := store.ListSingleUseLinks(ctx, filter)
		if nil != err {
			return 0, fmt.Errorf("failed to count active links: %w", err)
		}
		for _, link := range page {
			if "active" == link.Status && link.ExpiresAt.After(now) {
				active++
			}
		}
		if "" == next {
			break
		}
		filter.Cursor = next
	}
	metrics.SingleUseLinksActive.Set(float64(active))
	return active, nil
}
