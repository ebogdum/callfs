package natsmeta

import (
	"context"
	"fmt"
	"time"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metadata/fsm"
)

func (s *Store) barrier(ctx context.Context) error {
	if config.NATSReadBarrierPublish == s.cfg.ReadBarrier {
		_, err := s.apply(ctx, fsm.Command{Op: "noop"})
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, s.applyTimeout)
	defer cancel()
	info, err := s.stream.Info(callCtx)
	if nil != err {
		return fmt.Errorf("natsmeta: read stream info: %w", err)
	}
	return s.WaitApplied(callCtx, info.State.LastSeq)
}

func (s *Store) barrierOnMiss(ctx context.Context) (bool, error) {
	if !s.cfg.MissBarrier || s.strictReads.Load() {
		return false, nil
	}
	if err := s.barrier(ctx); nil != err {
		return false, err
	}
	return true, nil
}

func (s *Store) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	if md, ok := s.machine.Get(path); ok {
		return md, nil
	}
	retry, err := s.barrierOnMiss(ctx)
	if nil != err {
		return nil, err
	}
	if retry {
		if md, ok := s.machine.Get(path); ok {
			return md, nil
		}
	}
	return nil, metadata.ErrNotFound
}

func (s *Store) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	return s.machine.ListByPrefix(prefix, startAfter, limit), nil
}

func (s *Store) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	return s.machine.ListChildren(parentPath), nil
}

func (s *Store) HasChildren(ctx context.Context, parentPath string) (bool, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return false, err
	}
	return s.machine.HasChildren(parentPath), nil
}

func (s *Store) CountChildren(ctx context.Context, parentPath string) (int, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return 0, err
	}
	return s.machine.CountChildren(parentPath), nil
}

func (s *Store) ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	if 0 > maxDepth {
		return nil, fmt.Errorf("max depth must not be negative: %d", maxDepth)
	}
	return s.machine.ListDescendants(dirPath, maxDepth, limit), nil
}

func (s *Store) SearchDescendants(ctx context.Context, dirPath string, maxDepth, limit int, match func(*metadata.Metadata) bool) ([]*metadata.Metadata, int, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, 0, err
	}
	if 0 > maxDepth {
		return nil, 0, fmt.Errorf("max depth must not be negative: %d", maxDepth)
	}
	matched, scanned := s.machine.SearchDescendants(dirPath, maxDepth, limit, match)
	return matched, scanned, nil
}

func (s *Store) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	if link, ok := s.machine.GetLink(token); ok {
		return link, nil
	}
	retry, err := s.barrierOnMiss(ctx)
	if nil != err {
		return nil, err
	}
	if retry {
		if link, ok := s.machine.GetLink(token); ok {
			return link, nil
		}
	}
	return nil, metadata.ErrNotFound
}

func (s *Store) GetSingleUseLinkByID(ctx context.Context, id int64) (*metadata.SingleUseLink, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	if link, ok := s.machine.LinkByID(id, time.Now().UTC()); ok {
		return link, nil
	}
	retry, err := s.barrierOnMiss(ctx)
	if nil != err {
		return nil, err
	}
	if retry {
		if link, ok := s.machine.LinkByID(id, time.Now().UTC()); ok {
			return link, nil
		}
	}
	return nil, metadata.ErrNotFound
}

func (s *Store) CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return 0, err
	}
	return s.machine.CountActiveLinks(owner, now), nil
}

func (s *Store) ListSingleUseLinks(ctx context.Context, filter metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, "", err
	}
	return s.machine.ListLinks(filter)
}

func (s *Store) OwnerStats(ctx context.Context, cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, "", err
	}
	return s.machine.OwnerStats(cursor, limit)
}

func (s *Store) SearchMetadata(ctx context.Context, q metadata.SearchQuery) (metadata.SearchResult, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return metadata.SearchResult{}, err
	}
	return s.machine.Search(ctx, q)
}

func (s *Store) CountEmptyETags(ctx context.Context) (int, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return 0, err
	}
	return s.machine.CountEmptyETags(), nil
}

func (s *Store) CountEmptyOwners(ctx context.Context) (int, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return 0, err
	}
	return s.machine.CountEmptyOwners(), nil
}

func (s *Store) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, err
	}
	if info, ok := s.machine.GetErasureInfo(filePath); ok {
		return info, nil
	}
	retry, err := s.barrierOnMiss(ctx)
	if nil != err {
		return nil, err
	}
	if retry {
		if info, ok := s.machine.GetErasureInfo(filePath); ok {
			return info, nil
		}
	}
	return nil, metadata.ErrNotFound
}

func (s *Store) ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error) {
	if err := s.strictBarrier(ctx); nil != err {
		return nil, "", err
	}
	return s.machine.ListErasureInfos(prefix, cursor, limit)
}
