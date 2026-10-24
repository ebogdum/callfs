package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	usageCacheTTL        = time.Minute
	usageCacheMaxEntries = 256
	usageSyncRowLimit    = 200000
	usageJobRowLimit     = 2000000
	usageMaxDepth        = 3
	usageMaxChildren     = 1000
	usageMaxOwnerBytes   = 256
)

var (
	usageSizeBounds    = []int64{1024, 65536, 1048576, 16777216, 268435456, 4294967296, 68719476736}
	usageAgeBoundsDays = []int64{1, 7, 30, 90, 365}
)

type UsageTotals struct {
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}

type UsageChild struct {
	Path  string `json:"path"`
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

type UsageSizeBucket struct {
	Le    *int64 `json:"le"`
	Count int64  `json:"count"`
}

type UsageAgeBucket struct {
	LeDays *int64 `json:"le_days"`
	Count  int64  `json:"count"`
}

type Usage struct {
	Path              string                 `json:"path"`
	Files             int64                  `json:"files"`
	Directories       int64                  `json:"directories"`
	Bytes             int64                  `json:"bytes"`
	ByStorage         map[string]UsageTotals `json:"by_storage"`
	ByInstance        map[string]UsageTotals `json:"by_instance"`
	ByOwner           map[string]UsageTotals `json:"by_owner"`
	ErasureFiles      int64                  `json:"erasure_files"`
	ErasureRawBytes   int64                  `json:"erasure_raw_bytes"`
	Children          []UsageChild           `json:"children"`
	ChildrenTruncated bool                   `json:"children_truncated"`
	SizeHistogram     []UsageSizeBucket      `json:"size_histogram,omitempty"`
	AgeHistogram      []UsageAgeBucket       `json:"age_histogram,omitempty"`
	ComputedAt        string                 `json:"computed_at"`
	ScanTruncated     bool                   `json:"scan_truncated"`
}

type usageAcceptedResponse struct {
	JobID  string `json:"job_id"`
	Result *Usage `json:"result,omitempty"`
}

type usageKey struct {
	path  string
	depth int
	owner string
}

type usageEntry struct {
	usage     Usage
	expiresAt time.Time
}

type UsageCache struct {
	mu      sync.Mutex
	entries map[usageKey]usageEntry
	runner  *jobs.Runner
}

func NewUsageCache(runner *jobs.Runner) *UsageCache {
	return &UsageCache{entries: make(map[usageKey]usageEntry), runner: runner}
}

func (c *UsageCache) get(key usageKey) (usageEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	return entry, ok
}

func (c *UsageCache) put(key usageKey, usage Usage, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && usageCacheMaxEntries <= len(c.entries) {
		for evicted := range c.entries {
			delete(c.entries, evicted)
			break
		}
	}
	c.entries[key] = usageEntry{usage: usage, expiresAt: expiresAt}
}

func V1Usage(engine *core.Engine, usageCache *UsageCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		query := r.URL.Query()
		root, ok := normalizeQueryPath(query.Get("path"))
		if !ok {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}

		depth := 0
		if raw := query.Get("depth"); "" != raw {
			parsed, err := strconv.Atoi(raw)
			if nil != err || 0 > parsed || usageMaxDepth < parsed {
				SendErrorResponse(w, logger, &customError{message: "depth must be 0 to 3"}, http.StatusBadRequest)
				return
			}
			depth = parsed
		}

		histograms, err := parseOptionalBool(query, "histograms")
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		includeHistograms := nil != histograms && *histograms

		owner, err := boundedQueryValue(query, "owner", usageMaxOwnerBytes)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}

		key := usageKey{path: root, depth: depth, owner: owner}
		now := time.Now()
		if entry, found := usageCache.get(key); found && now.Before(entry.expiresAt) {
			SendJSONResponse(w, usageForResponse(entry.usage, includeHistograms))
			return
		}

		md, err := engine.GetMetadata(r.Context(), root)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		if "directory" != md.Type {
			SendErrorResponse(w, logger, &customError{message: "path is not a directory"}, http.StatusBadRequest)
			return
		}

		if nil == usageCache.runner {
			usage, _, err := computeUsage(r.Context(), engine, root, owner, depth, 0)
			if nil != err {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
			usageCache.put(key, usage, now.Add(usageCacheTTL))
			SendJSONResponse(w, usageForResponse(usage, includeHistograms))
			return
		}

		usage, rowCount, err := computeUsage(r.Context(), engine, root, owner, depth, usageSyncRowLimit+1)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		if rowCount <= usageSyncRowLimit {
			usageCache.put(key, usage, now.Add(usageCacheTTL))
			SendJSONResponse(w, usageForResponse(usage, includeHistograms))
			return
		}

		job := usageCache.runner.Submit("usage_scan", userID, func(ctx context.Context, _ *jobs.Progress) (any, error) {
			logger.Info("usage scan started", zap.String("path", root), zap.Int("depth", depth))
			full, scanned, scanErr := computeUsage(ctx, engine, root, owner, depth, usageJobRowLimit)
			if nil != scanErr {
				logger.Error("usage scan failed", zap.String("path", root), zap.Error(scanErr))
				return nil, scanErr
			}
			full.ScanTruncated = usageJobRowLimit == scanned
			usageCache.put(key, full, time.Now().Add(usageCacheTTL))
			logger.Info("usage scan finished", zap.String("path", root), zap.Int("rows", scanned))
			return full, nil
		})

		accepted := usageAcceptedResponse{JobID: job.ID}
		if entry, found := usageCache.get(key); found {
			result := usageForResponse(entry.usage, includeHistograms)
			accepted.Result = &result
		}
		sendAcceptedResponse(w, logger, accepted)
	}
}

func usageForResponse(usage Usage, includeHistograms bool) Usage {
	if !includeHistograms {
		usage.SizeHistogram = nil
		usage.AgeHistogram = nil
	}
	return usage
}

func computeUsage(ctx context.Context, engine *core.Engine, root, owner string, depth, limit int) (Usage, int, error) {
	rows, err := engine.ListDirectoryRecursiveLimited(ctx, root, -1, limit)
	if nil != err {
		return Usage{}, 0, err
	}
	usage := aggregateUsage(rows, root, depth, owner, engine.GetCurrentInstanceID(), time.Now())
	return usage, len(rows), nil
}

func aggregateUsage(rows []*metadata.Metadata, root string, depth int, owner, currentInstanceID string, now time.Time) Usage {
	usage := Usage{
		Path:       root,
		ByStorage:  make(map[string]UsageTotals),
		ByInstance: make(map[string]UsageTotals),
		ByOwner:    make(map[string]UsageTotals),
		Children:   []UsageChild{},
		ComputedAt: rfc3339UTC(now),
	}
	sizeCounts := make([]int64, len(usageSizeBounds)+1)
	ageCounts := make([]int64, len(usageAgeBoundsDays)+1)
	children := make(map[string]*UsageChild)

	for _, md := range rows {
		if "" != owner && owner != md.Owner {
			continue
		}
		segments := relativeSegments(root, md.Path)
		if "directory" == md.Type {
			usage.Directories++
			if 0 < depth && depth == len(segments) {
				ensureUsageChild(children, root, segments)
			}
			continue
		}

		usage.Files++
		usage.Bytes += md.Size
		addUsageTotals(usage.ByStorage, md.BackendType, md.Size)
		addUsageTotals(usage.ByInstance, owningInstanceID(md, currentInstanceID), md.Size)
		addUsageTotals(usage.ByOwner, md.Owner, md.Size)
		if md.ErasureCoded {
			usage.ErasureFiles++
			usage.ErasureRawBytes += md.Size
		}
		sizeCounts[usageSizeBucket(md.Size)]++
		ageCounts[usageAgeBucket(now.Sub(md.MTime))]++

		if 0 < depth && depth < len(segments) {
			child := ensureUsageChild(children, root, segments[:depth])
			child.Files++
			child.Bytes += md.Size
		}
	}

	usage.Children, usage.ChildrenTruncated = sortedUsageChildren(children)
	usage.SizeHistogram = sizeHistogram(sizeCounts)
	usage.AgeHistogram = ageHistogram(ageCounts)
	return usage
}

func relativeSegments(root, entryPath string) []string {
	rest := strings.TrimPrefix(strings.TrimPrefix(entryPath, root), "/")
	if "" == rest {
		return nil
	}
	return strings.Split(rest, "/")
}

func ensureUsageChild(children map[string]*UsageChild, root string, segments []string) *UsageChild {
	childPath := path.Join(append([]string{root}, segments...)...)
	child, found := children[childPath]
	if !found {
		child = &UsageChild{Path: childPath}
		children[childPath] = child
	}
	return child
}

func sortedUsageChildren(children map[string]*UsageChild) ([]UsageChild, bool) {
	sorted := make([]UsageChild, 0, len(children))
	for _, child := range children {
		sorted = append(sorted, *child)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	if usageMaxChildren < len(sorted) {
		return sorted[:usageMaxChildren], true
	}
	return sorted, false
}

func addUsageTotals(totals map[string]UsageTotals, key string, size int64) {
	entry := totals[key]
	entry.Files++
	entry.Bytes += size
	totals[key] = entry
}

func usageSizeBucket(size int64) int {
	for index, bound := range usageSizeBounds {
		if size <= bound {
			return index
		}
	}
	return len(usageSizeBounds)
}

func usageAgeBucket(age time.Duration) int {
	for index, days := range usageAgeBoundsDays {
		if age <= time.Duration(days)*24*time.Hour {
			return index
		}
	}
	return len(usageAgeBoundsDays)
}

func sizeHistogram(counts []int64) []UsageSizeBucket {
	buckets := make([]UsageSizeBucket, 0, len(counts))
	for index, bound := range usageSizeBounds {
		le := bound
		buckets = append(buckets, UsageSizeBucket{Le: &le, Count: counts[index]})
	}
	return append(buckets, UsageSizeBucket{Count: counts[len(usageSizeBounds)]})
}

func ageHistogram(counts []int64) []UsageAgeBucket {
	buckets := make([]UsageAgeBucket, 0, len(counts))
	for index, days := range usageAgeBoundsDays {
		le := days
		buckets = append(buckets, UsageAgeBucket{LeDays: &le, Count: counts[index]})
	}
	return append(buckets, UsageAgeBucket{Count: counts[len(usageAgeBoundsDays)]})
}

func sendAcceptedResponse(w http.ResponseWriter, logger *zap.Logger, body any) {
	buf, err := json.Marshal(body)
	if nil != err {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if _, writeErr := w.Write(append(buf, '\n')); nil != writeErr {
		logger.Warn("failed to write accepted response", zap.Error(writeErr))
	}
}
