package erasure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

type ShardState string

const (
	ShardOK          ShardState = "ok"
	ShardMissing     ShardState = "missing"
	ShardCorrupt     ShardState = "corrupt"
	ShardUnreachable ShardState = "unreachable"
)

type Health string

const (
	HealthOK         Health = "ok"
	HealthReduced    Health = "reduced"
	HealthCritical   Health = "critical"
	HealthUnreadable Health = "unreadable"
	HealthUnknown    Health = "unknown"
)

type ShardVerdict struct {
	Index int        `json:"index"`
	State ShardState `json:"state"`
	Error *string    `json:"error"`
}

type VerifyReport struct {
	Path     string         `json:"path"`
	Readable bool           `json:"readable"`
	Healthy  int            `json:"healthy"`
	Required int            `json:"required"`
	Shards   []ShardVerdict `json:"shards"`
}

type FileHealth struct {
	Path          string    `json:"path"`
	Health        Health    `json:"health"`
	HealthyShards int       `json:"healthy_shards"`
	ShardsTotal   int       `json:"shards_total"`
	CheckedAt     time.Time `json:"last_checked_at"`
}

type PlanAssignment struct {
	Index      int    `json:"index"`
	InstanceID string `json:"instance_id"`
	Kind       string `json:"kind"`
}

type Plan struct {
	TotalShards               int              `json:"total_shards"`
	Assignments               []PlanAssignment `json:"assignments"`
	PerInstance               map[string]int   `json:"per_instance"`
	MaxOnOneInstance          int              `json:"max_on_one_instance"`
	ToleratedInstanceFailures int              `json:"tolerated_instance_failures"`
	ToleratedPieceLosses      int              `json:"tolerated_piece_losses"`
	OverheadRatio             float64          `json:"overhead_ratio"`
	FaultTolerant             bool             `json:"fault_tolerant"`
	Warnings                  []string         `json:"warnings"`
}

func (m *Manager) Verify(ctx context.Context, path string) (VerifyReport, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return VerifyReport{}, fmt.Errorf("failed to get erasure info: %w", err)
	}

	verdicts := make([]ShardVerdict, len(mdInfo.Shards))
	var wg sync.WaitGroup
	for i, si := range mdInfo.Shards {
		wg.Add(1)
		go func(i int, si metadata.ErasureShardInfo) {
			defer wg.Done()
			verdicts[i] = m.verifyShard(ctx, si)
		}(i, si)
	}
	wg.Wait()

	if ctxErr := ctx.Err(); nil != ctxErr {
		return VerifyReport{}, ctxErr
	}

	report := VerifyReport{Path: path, Required: mdInfo.DataShards, Shards: verdicts}
	for _, verdict := range verdicts {
		if ShardOK == verdict.State {
			report.Healthy++
		}
	}
	slices.SortFunc(report.Shards, func(a, b ShardVerdict) int { return a.Index - b.Index })
	report.Readable = report.Healthy >= report.Required

	total := mdInfo.DataShards + mdInfo.ParityShards
	m.storeHealth(FileHealth{
		Path:          path,
		Health:        classifyHealth(report.Readable, report.Healthy, report.Required, total),
		HealthyShards: report.Healthy,
		ShardsTotal:   total,
		CheckedAt:     time.Now().UTC(),
	})
	return report, nil
}

func (m *Manager) verifyShard(ctx context.Context, si metadata.ErasureShardInfo) ShardVerdict {
	data, err := m.fetchShard(ctx, si)
	switch {
	case nil == err && ShardChecksum(data) == si.Checksum:
		return ShardVerdict{Index: si.Index, State: ShardOK}
	case nil == err, errors.Is(err, io.ErrUnexpectedEOF):
		return newShardVerdict(si.Index, ShardCorrupt, ErrShardCorrupted)
	case errors.Is(err, metadata.ErrNotFound):
		return newShardVerdict(si.Index, ShardMissing, err)
	default:
		return newShardVerdict(si.Index, ShardUnreachable, err)
	}
}

func newShardVerdict(index int, state ShardState, err error) ShardVerdict {
	verdict := ShardVerdict{Index: index, State: state}
	if nil != err {
		message := err.Error()
		verdict.Error = &message
	}
	return verdict
}

func classifyHealth(readable bool, healthy, required, total int) Health {
	switch {
	case !readable:
		return HealthUnreadable
	case healthy >= total:
		return HealthOK
	case healthy == required:
		return HealthCritical
	default:
		return HealthReduced
	}
}

func (m *Manager) LastHealth(path string) (FileHealth, bool) {
	m.healthMu.RLock()
	defer m.healthMu.RUnlock()
	health, ok := m.healthCache[path]
	return health, ok
}

func (m *Manager) storeHealth(health FileHealth) {
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	m.healthCache[health.Path] = health
}

func (m *Manager) forgetHealth(path string) {
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	delete(m.healthCache, path)
}

func (m *Manager) RemovePeer(instanceID string) {
	m.peerMu.Lock()
	defer m.peerMu.Unlock()
	delete(m.peerEndpoints, instanceID)
}

func (m *Manager) Plan(dataShards, parityShards int, instances []string) Plan {
	plan := Plan{
		PerInstance: map[string]int{},
		Assignments: []PlanAssignment{},
		Warnings:    []string{},
	}
	if dataShards < 1 || parityShards < 1 {
		plan.Warnings = append(plan.Warnings, "data and spare pieces must both be at least 1")
		return plan
	}

	total := dataShards + parityShards
	owners := m.placement.AssignShards(total, m.instanceID, m.resolveInstances(&StoreOptions{Instances: instances}))
	plan.TotalShards = total
	plan.OverheadRatio = float64(total) / float64(dataShards)
	for index, instanceID := range owners {
		kind := "parity"
		if index < dataShards {
			kind = "data"
		}
		plan.Assignments = append(plan.Assignments, PlanAssignment{Index: index, InstanceID: instanceID, Kind: kind})
		plan.PerInstance[instanceID]++
	}

	counts := make([]int, 0, len(plan.PerInstance))
	for _, count := range plan.PerInstance {
		counts = append(counts, count)
	}
	slices.SortFunc(counts, func(a, b int) int { return b - a })
	plan.MaxOnOneInstance = counts[0]
	lost := 0
	for _, count := range counts {
		if lost+count > parityShards {
			break
		}
		lost += count
		plan.ToleratedInstanceFailures++
	}
	plan.ToleratedPieceLosses = parityShards
	plan.FaultTolerant = plan.ToleratedInstanceFailures >= 1

	if !plan.FaultTolerant {
		plan.Warnings = append(plan.Warnings, "One server failure can break split files")
	}
	if 1 == len(plan.PerInstance) {
		plan.Warnings = append(plan.Warnings, "All pieces would be on one server")
	}
	return plan
}
