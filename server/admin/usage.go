package admin

import (
	"context"
	"time"

	"github.com/ebogdum/callfs/metrics"
)

const usageRefreshRoot = "/"

type usageRefreshResult struct {
	Path        string    `json:"path"`
	Files       int64     `json:"files"`
	Bytes       int64     `json:"bytes"`
	RefreshedAt time.Time `json:"refreshed_at"`
}

type usageTotals struct {
	files int64
	bytes int64
}

func (a *API) UsageRefreshWorker(ctx context.Context) (any, error) {
	result := usageRefreshResult{Path: usageRefreshRoot}
	byBackend := make(map[string]usageTotals)
	startAfter := ""
	for {
		if err := ctx.Err(); nil != err {
			return nil, err
		}
		files, err := a.deps.Store.ListByPrefix(ctx, usageRefreshRoot, startAfter, instancePageSize)
		if nil != err {
			return nil, err
		}
		for _, md := range files {
			result.Files++
			result.Bytes += md.Size
			totals := byBackend[md.BackendType]
			totals.files++
			totals.bytes += md.Size
			byBackend[md.BackendType] = totals
		}
		if instancePageSize > len(files) {
			break
		}
		startAfter = files[len(files)-1].Path
	}
	for storage, totals := range byBackend {
		metrics.UsageFiles.WithLabelValues(storage).Set(float64(totals.files))
		metrics.UsageBytes.WithLabelValues(storage).Set(float64(totals.bytes))
	}
	result.RefreshedAt = time.Now().UTC()
	return result, nil
}
