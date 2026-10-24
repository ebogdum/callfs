package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"sort"
	"sync"
	"time"
)

const reservoirSize = 50000

type opStats struct {
	count   int64
	errors  int64
	bytes   int64
	total   time.Duration
	max     time.Duration
	samples []time.Duration
	seen    int64
	rand    *rand.Rand
	lastErr string
}

type Recorder struct {
	mu  sync.Mutex
	ops map[string]*opStats
}

type OpSummary struct {
	Op        string  `json:"op"`
	Count     int64   `json:"count"`
	Errors    int64   `json:"errors"`
	Bytes     int64   `json:"bytes"`
	PerSecond float64 `json:"per_second"`
	MBPerSec  float64 `json:"mb_per_second"`
	MeanMs    float64 `json:"mean_ms"`
	P50Ms     float64 `json:"p50_ms"`
	P95Ms     float64 `json:"p95_ms"`
	P99Ms     float64 `json:"p99_ms"`
	MaxMs     float64 `json:"max_ms"`
	LastError string  `json:"last_error,omitempty"`
}

func NewRecorder() *Recorder {
	return &Recorder{ops: map[string]*opStats{}}
}

func (r *Recorder) Record(op string, took time.Duration, bytes int64, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.ops[op]
	if !ok {
		s = &opStats{rand: rand.New(rand.NewPCG(uint64(len(r.ops))+1, 7))}
		r.ops[op] = s
	}
	s.count++
	s.bytes += bytes
	s.total += took
	if took > s.max {
		s.max = took
	}
	if nil != err {
		s.errors++
		s.lastErr = err.Error()
	}
	s.seen++
	if reservoirSize > len(s.samples) {
		s.samples = append(s.samples, took)
		return
	}
	if slot := s.rand.Int64N(s.seen); reservoirSize > slot {
		s.samples[slot] = took
	}
}

func (r *Recorder) Summaries(elapsed time.Duration) []OpSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]OpSummary, 0, len(r.ops))
	seconds := elapsed.Seconds()
	if 0 >= seconds {
		seconds = 1
	}
	for op, s := range r.ops {
		sorted := slices.Clone(s.samples)
		slices.Sort(sorted)
		out = append(out, OpSummary{
			Op:        op,
			Count:     s.count,
			Errors:    s.errors,
			Bytes:     s.bytes,
			PerSecond: float64(s.count) / seconds,
			MBPerSec:  float64(s.bytes) / seconds / (1 << 20),
			MeanMs:    ms(s.total) / float64(max(s.count, 1)),
			P50Ms:     ms(percentile(sorted, 0.50)),
			P95Ms:     ms(percentile(sorted, 0.95)),
			P99Ms:     ms(percentile(sorted, 0.99)),
			MaxMs:     ms(s.max),
			LastError: s.lastErr,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].P99Ms > out[j].P99Ms })
	return out
}

func percentile(sorted []time.Duration, q float64) time.Duration {
	if 0 == len(sorted) {
		return 0
	}
	idx := int(q * float64(len(sorted)-1))
	return sorted[idx]
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func writeTable(w io.Writer, title string, rows []OpSummary) error {
	if _, err := fmt.Fprintf(w, "\n== %s ==\n%-34s %8s %6s %9s %8s %9s %9s %9s %9s\n", title, "op", "count", "errs", "ops/s", "MB/s", "p50ms", "p95ms", "p99ms", "maxms"); nil != err {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-34s %8d %6d %9.1f %8.2f %9.2f %9.2f %9.2f %9.2f\n", row.Op, row.Count, row.Errors, row.PerSecond, row.MBPerSec, row.P50Ms, row.P95Ms, row.P99Ms, row.MaxMs); nil != err {
			return err
		}
		if "" != row.LastError {
			if _, err := fmt.Fprintf(w, "    last error: %s\n", row.LastError); nil != err {
				return err
			}
		}
	}
	return nil
}
