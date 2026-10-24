package links

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

type countingStore struct {
	metadata.Store
	active  int
	created int
}

func (s *countingStore) CountActiveSingleUseLinks(_ context.Context, _ string, _ time.Time) (int, error) {
	return s.active, nil
}

func (s *countingStore) CreateSingleUseLink(_ context.Context, _ *metadata.SingleUseLink) error {
	s.created++
	return nil
}

func TestNewLinkManagerRejectsNegativeLimit(t *testing.T) {
	if _, err := NewLinkManager(&countingStore{}, "secret", -1, zap.NewNop()); nil == err {
		t.Fatalf("negative limit must be rejected")
	}
}

func TestGenerateOwnedLinkHonorsConfiguredLimit(t *testing.T) {
	store := &countingStore{active: 2}
	manager, err := NewLinkManager(store, "secret", 3, zap.NewNop())
	if nil != err {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.GenerateOwnedLink(context.Background(), "alice", "/f", time.Minute); nil != err {
		t.Fatalf("below limit: %v", err)
	}
	store.active = 3
	if _, err := manager.GenerateOwnedLink(context.Background(), "alice", "/f", time.Minute); !errors.Is(err, ErrTooManyActiveLinks) {
		t.Fatalf("at limit: got %v, want ErrTooManyActiveLinks", err)
	}
	if 1 != store.created {
		t.Fatalf("created %d links, want 1", store.created)
	}
}

func TestZeroLimitUsesDefault(t *testing.T) {
	store := &countingStore{active: DefaultMaxActiveLinksPerOwner - 1}
	manager, err := NewLinkManager(store, "secret", 0, zap.NewNop())
	if nil != err {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.GenerateOwnedLink(context.Background(), "alice", "/f", time.Minute); nil != err {
		t.Fatalf("below default: %v", err)
	}
	store.active = DefaultMaxActiveLinksPerOwner
	if _, err := manager.GenerateOwnedLink(context.Background(), "alice", "/f", time.Minute); !errors.Is(err, ErrTooManyActiveLinks) {
		t.Fatalf("at default: got %v, want ErrTooManyActiveLinks", err)
	}
}
