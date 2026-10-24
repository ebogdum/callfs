package metadata

import (
	"context"
	"time"

	"github.com/ebogdum/callfs/v2/metrics"
)

type instrumentedStore struct {
	Store
}

func Instrument(store Store) Store {
	if nil == store {
		return nil
	}
	return &instrumentedStore{Store: store}
}

func (s *instrumentedStore) LocalState() bool {
	localState, ok := s.Store.(LocalState)
	return ok && localState.LocalState()
}

func observeMetadataQuery(operation string, start time.Time) {
	metrics.MetadataDBQueriesTotal.WithLabelValues(operation).Inc()
	metrics.MetadataDBQueryDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}

func (s *instrumentedStore) Get(ctx context.Context, path string) (*Metadata, error) {
	defer observeMetadataQuery("get", time.Now())
	return s.Store.Get(ctx, path)
}

func (s *instrumentedStore) Create(ctx context.Context, md *Metadata) error {
	defer observeMetadataQuery("create", time.Now())
	return s.Store.Create(ctx, md)
}

func (s *instrumentedStore) Update(ctx context.Context, md *Metadata) error {
	defer observeMetadataQuery("update", time.Now())
	return s.Store.Update(ctx, md)
}

func (s *instrumentedStore) Delete(ctx context.Context, path string) error {
	defer observeMetadataQuery("delete", time.Now())
	return s.Store.Delete(ctx, path)
}

func (s *instrumentedStore) Rename(ctx context.Context, oldPath, newPath string) error {
	defer observeMetadataQuery("rename", time.Now())
	return s.Store.Rename(ctx, oldPath, newPath)
}

func (s *instrumentedStore) UpdateIfMatch(ctx context.Context, md *Metadata, expectedETag string) error {
	defer observeMetadataQuery("update_if_match", time.Now())
	return s.Store.UpdateIfMatch(ctx, md, expectedETag)
}

func (s *instrumentedStore) ListChildren(ctx context.Context, parentPath string) ([]*Metadata, error) {
	defer observeMetadataQuery("list_children", time.Now())
	return s.Store.ListChildren(ctx, parentPath)
}

func (s *instrumentedStore) CountChildren(ctx context.Context, parentPath string) (int, error) {
	defer observeMetadataQuery("count_children", time.Now())
	if counter, ok := s.Store.(ChildCounter); ok {
		return counter.CountChildren(ctx, parentPath)
	}
	children, err := s.Store.ListChildren(ctx, parentPath)
	if nil != err {
		return 0, err
	}
	return len(children), nil
}

func (s *instrumentedStore) HasChildren(ctx context.Context, parentPath string) (bool, error) {
	defer observeMetadataQuery("has_children", time.Now())
	return s.Store.HasChildren(ctx, parentPath)
}

func (s *instrumentedStore) ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*Metadata, error) {
	defer observeMetadataQuery("list_descendants", time.Now())
	return s.Store.ListDescendants(ctx, dirPath, maxDepth, limit)
}

func (s *instrumentedStore) SearchDescendants(ctx context.Context, dirPath string, maxDepth, limit int, match func(*Metadata) bool) ([]*Metadata, int, error) {
	defer observeMetadataQuery("list_descendants", time.Now())
	return SearchDescendants(ctx, s.Store, dirPath, maxDepth, limit, match)
}

func (s *instrumentedStore) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*Metadata, error) {
	defer observeMetadataQuery("list_by_prefix", time.Now())
	return s.Store.ListByPrefix(ctx, prefix, startAfter, limit)
}

func (s *instrumentedStore) GetSingleUseLink(ctx context.Context, token string) (*SingleUseLink, error) {
	defer observeMetadataQuery("get_single_use_link", time.Now())
	return s.Store.GetSingleUseLink(ctx, token)
}

func (s *instrumentedStore) CreateSingleUseLink(ctx context.Context, link *SingleUseLink) error {
	defer observeMetadataQuery("create_single_use_link", time.Now())
	return s.Store.CreateSingleUseLink(ctx, link)
}

func (s *instrumentedStore) CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error) {
	defer observeMetadataQuery("count_active_single_use_links", time.Now())
	return s.Store.CountActiveSingleUseLinks(ctx, owner, now)
}

func (s *instrumentedStore) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	defer observeMetadataQuery("update_single_use_link", time.Now())
	return s.Store.UpdateSingleUseLink(ctx, token, status, usedAt, usedByIP)
}

func (s *instrumentedStore) ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*SingleUseLink, bool, error) {
	defer observeMetadataQuery("consume_single_use_link", time.Now())
	return s.Store.ConsumeSingleUseLink(ctx, token, usedByIP, now)
}

func (s *instrumentedStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	defer observeMetadataQuery("cleanup_expired_links", time.Now())
	return s.Store.CleanupExpiredLinks(ctx, before)
}

func (s *instrumentedStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	defer observeMetadataQuery("cleanup_used_links", time.Now())
	return s.Store.CleanupUsedLinks(ctx, olderThan)
}

func (s *instrumentedStore) Close() error {
	defer observeMetadataQuery("close", time.Now())
	return s.Store.Close()
}

func (s *instrumentedStore) ApplyBatch(ctx context.Context, ops []Op) error {
	defer observeMetadataQuery("apply_batch", time.Now())
	return ApplyBatch(ctx, s.Store, ops)
}

func (s *instrumentedStore) AtomicNamespace() bool {
	atomic, ok := s.Store.(AtomicNamespace)
	return ok && atomic.AtomicNamespace()
}
