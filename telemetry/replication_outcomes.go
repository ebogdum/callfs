package telemetry

import (
	"sync"
	"time"
)

const outcomeBuckets = 24 * 60

const (
	ReplicationSucceeded = "succeeded"
	ReplicationSkipped   = "skipped"
	ReplicationFailed    = "failed"
	ReplicationDropped   = "dropped"
)

type outcomeBucket struct {
	minute  int64
	ok      int
	skipped int
	failed  int
}

type ReplicationOutcomes struct {
	mu      sync.Mutex
	buckets [outcomeBuckets]outcomeBucket
}

func NewReplicationOutcomes() *ReplicationOutcomes {
	return &ReplicationOutcomes{}
}

func (r *ReplicationOutcomes) Record(outcome string) {
	minute := time.Now().Unix() / 60
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket := &r.buckets[minute%outcomeBuckets]
	if minute != bucket.minute {
		*bucket = outcomeBucket{minute: minute}
	}
	switch outcome {
	case ReplicationSucceeded:
		bucket.ok++
	case ReplicationSkipped:
		bucket.skipped++
	case ReplicationFailed, ReplicationDropped:
		bucket.failed++
	}
}

func (r *ReplicationOutcomes) Since(since time.Time) (ok, skipped, failed int) {
	from := since.Unix() / 60
	now := time.Now().Unix() / 60
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.buckets {
		bucket := r.buckets[i]
		if from <= bucket.minute && now >= bucket.minute {
			ok += bucket.ok
			skipped += bucket.skipped
			failed += bucket.failed
		}
	}
	return ok, skipped, failed
}
