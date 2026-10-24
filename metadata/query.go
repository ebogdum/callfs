package metadata

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

var ErrInvalidQuery = errors.New("invalid metadata query")

const (
	SearchScanBudget = 50000
	searchPageSize   = 500
	searchMaxLimit   = 1000
)

type LinkFilter struct {
	Status    string    `json:"status"`
	FilePath  string    `json:"file_path"`
	CreatedBy string    `json:"created_by"`
	Now       time.Time `json:"now"`
	Limit     int       `json:"limit"`
	Cursor    string    `json:"cursor"`
}

type SearchQuery struct {
	Q              string     `json:"q"`
	Path           string     `json:"path"`
	Type           string     `json:"type"`
	MinSize        *int64     `json:"min_size"`
	MaxSize        *int64     `json:"max_size"`
	ModifiedAfter  *time.Time `json:"modified_after"`
	ModifiedBefore *time.Time `json:"modified_before"`
	Owner          string     `json:"owner"`
	Storage        string     `json:"storage"`
	Instance       string     `json:"instance"`
	Erasure        *bool      `json:"erasure"`
	Limit          int        `json:"limit"`
	Cursor         string     `json:"cursor"`
}

type SearchResult struct {
	Items         []*Metadata               `json:"items"`
	NextCursor    string                    `json:"next_cursor"`
	ScanTruncated bool                      `json:"scan_truncated"`
	Scanned       int                       `json:"scanned"`
	Facets        map[string]map[string]int `json:"facets"`
}

type OwnerStat struct {
	Owner string `json:"owner"`
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

type searchScan struct {
	store       Store
	query       SearchQuery
	result      SearchResult
	scanned     int
	lastScanned string
	inFiles     bool
	budgetHit   bool
}

func SearchByScan(ctx context.Context, store Store, q SearchQuery) (SearchResult, error) {
	if err := validateSearchQuery(q); err != nil {
		return SearchResult{}, err
	}
	scan := &searchScan{
		store: store,
		query: q,
		result: SearchResult{
			Items: make([]*Metadata, 0, q.Limit+1),
			Facets: map[string]map[string]int{
				"type":     {},
				"storage":  {},
				"owner":    {},
				"instance": {},
			},
		},
	}
	scope := normalizeSearchScope(q.Path)
	if "" == q.Cursor && "file" != q.Type {
		if err := scan.walkDirectories(ctx, scope); err != nil {
			return SearchResult{}, err
		}
	}
	if "directory" != q.Type {
		if len(scan.result.Items) < q.Limit {
			if err := scan.scanFiles(ctx, scope); err != nil {
				return SearchResult{}, err
			}
		} else {
			scan.result.ScanTruncated = true
		}
	}
	scan.finish()
	return scan.result, nil
}

func validateSearchQuery(q SearchQuery) error {
	if 0 >= q.Limit || searchMaxLimit < q.Limit {
		return ErrInvalidQuery
	}
	if "" != q.Type && "file" != q.Type && "directory" != q.Type {
		return ErrInvalidQuery
	}
	if strings.ContainsAny(q.Q, "*?[") {
		if _, err := path.Match(q.Q, ""); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidQuery, err)
		}
	}
	return nil
}

func normalizeSearchScope(p string) string {
	return path.Clean("/" + p)
}

func scopePrefix(scope string) string {
	if "/" == scope {
		return scope
	}
	return scope + "/"
}

func (s *searchScan) walkDirectories(ctx context.Context, root string) error {
	queue := []string{root}
	for 0 < len(queue) {
		if s.scanned >= SearchScanBudget {
			s.budgetHit = true
			return nil
		}
		dir := queue[0]
		queue = queue[1:]
		children, err := s.store.ListChildren(ctx, dir)
		if err != nil {
			return fmt.Errorf("list children of %s: %w", dir, err)
		}
		for _, child := range children {
			s.scanned++
			if "directory" != child.Type {
				continue
			}
			queue = append(queue, child.Path)
			if err := s.consider(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *searchScan) scanFiles(ctx context.Context, scope string) error {
	s.inFiles = true
	prefix := scopePrefix(scope)
	after := s.query.Cursor
	for {
		if s.scanned >= SearchScanBudget {
			s.budgetHit = true
			return nil
		}
		page := searchPageSize
		if left := SearchScanBudget - s.scanned; left < page {
			page = left
		}
		rows, err := s.store.ListByPrefix(ctx, prefix, after, page)
		if err != nil {
			return fmt.Errorf("list files under %s: %w", prefix, err)
		}
		for _, md := range rows {
			s.scanned++
			s.lastScanned = md.Path
			if err := s.consider(md); err != nil {
				return err
			}
		}
		if len(rows) < page {
			return nil
		}
		after = rows[len(rows)-1].Path
	}
}

func (s *searchScan) consider(md *Metadata) error {
	ok, err := s.matches(md)
	if err != nil || !ok {
		return err
	}
	s.countFacets(md)
	if len(s.result.Items) <= s.query.Limit {
		s.result.Items = append(s.result.Items, md)
	}
	return nil
}

func (s *searchScan) finish() {
	limit := s.query.Limit
	if len(s.result.Items) > limit {
		s.result.Items = s.result.Items[:limit]
		if s.inFiles {
			s.result.NextCursor = s.result.Items[limit-1].Path
		} else {
			s.result.ScanTruncated = true
		}
	}
	if s.budgetHit {
		s.result.ScanTruncated = true
		if "" == s.result.NextCursor && s.inFiles {
			s.result.NextCursor = s.lastScanned
		}
	}
	s.result.Scanned = s.scanned
}

func (s *searchScan) matches(md *Metadata) (bool, error) {
	q := s.query
	if "" != q.Q {
		ok, err := matchesSearchName(md, q.Q)
		if err != nil || !ok {
			return false, err
		}
	}
	switch {
	case "" != q.Type && md.Type != q.Type:
		return false, nil
	case nil != q.MinSize && md.Size < *q.MinSize:
		return false, nil
	case nil != q.MaxSize && md.Size > *q.MaxSize:
		return false, nil
	case nil != q.ModifiedAfter && !md.MTime.After(*q.ModifiedAfter):
		return false, nil
	case nil != q.ModifiedBefore && !md.MTime.Before(*q.ModifiedBefore):
		return false, nil
	case "" != q.Owner && md.Owner != q.Owner:
		return false, nil
	case "" != q.Storage && md.BackendType != q.Storage:
		return false, nil
	case "" != q.Instance && instanceOf(md) != q.Instance:
		return false, nil
	case nil != q.Erasure && md.ErasureCoded != *q.Erasure:
		return false, nil
	}
	return true, nil
}

func matchesSearchName(md *Metadata, pattern string) (bool, error) {
	name := strings.ToLower(md.Name)
	lowered := strings.ToLower(pattern)
	if strings.ContainsAny(lowered, "*?[") {
		ok, err := path.Match(lowered, name)
		if err != nil {
			return false, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
		}
		return ok, nil
	}
	return strings.Contains(name, lowered), nil
}

func instanceOf(md *Metadata) string {
	if nil == md.CallFSInstanceID {
		return ""
	}
	return *md.CallFSInstanceID
}

func (s *searchScan) countFacets(md *Metadata) {
	s.result.Facets["type"][md.Type]++
	s.result.Facets["storage"][md.BackendType]++
	s.result.Facets["owner"][md.Owner]++
	s.result.Facets["instance"][instanceOf(md)]++
}
