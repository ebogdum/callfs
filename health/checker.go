package health

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/ebogdum/callfs/locks"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusWarn     Status = "warn"
	StatusDown     Status = "down"
	StatusDisabled Status = "disabled"
)

const (
	probeTimeout   = 2 * time.Second
	reportTTL      = 5 * time.Second
	certWarnWindow = 21 * 24 * time.Hour
	diskWarnRatio  = 0.10
	diskDownRatio  = 0.01
)

type Result struct {
	Status Status
	Detail string
}

type Probe struct {
	Name string
	Run  func(ctx context.Context) Result
}

type Component struct {
	Name      string    `json:"name"`
	Status    Status    `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	Detail    string    `json:"detail"`
	Since     time.Time `json:"since"`
}

type Report struct {
	Status     Status      `json:"status"`
	CheckedAt  time.Time   `json:"checked_at"`
	Components []Component `json:"components"`
}

type componentState struct {
	status Status
	since  time.Time
}

type Checker struct {
	probes []Probe
	clock  func() time.Time

	mu       sync.Mutex
	states   map[string]componentState
	cached   Report
	cachedAt time.Time
}

func NewChecker(probes []Probe) *Checker {
	return &Checker{probes: probes, clock: time.Now, states: make(map[string]componentState)}
}

func (c *Checker) Report(ctx context.Context, fresh bool) Report {
	c.mu.Lock()
	if !fresh && reportTTL > c.clock().Sub(c.cachedAt) {
		report := c.cached
		c.mu.Unlock()
		return report
	}
	c.mu.Unlock()

	components := make([]Component, len(c.probes))
	var wg sync.WaitGroup
	for i, probe := range c.probes {
		wg.Add(1)
		go func(i int, probe Probe) {
			defer wg.Done()
			components[i] = runProbe(ctx, probe)
		}(i, probe)
	}
	wg.Wait()

	now := c.clock()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range components {
		comp := &components[i]
		prev, known := c.states[comp.Name]
		if !known || prev.status != comp.Status {
			prev = componentState{status: comp.Status, since: now}
			c.states[comp.Name] = prev
		}
		comp.Since = prev.since
	}
	report := Report{Status: overallStatus(components), CheckedAt: now, Components: components}
	c.cached = report
	c.cachedAt = now
	return report
}

func runProbe(ctx context.Context, probe Probe) Component {
	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	done := make(chan Result, 1)
	go func() {
		defer func() {
			if recovered := recover(); nil != recovered {
				done <- Result{Status: StatusDown, Detail: "Check failed unexpectedly"}
			}
		}()
		done <- probe.Run(probeCtx)
	}()
	result := Result{Status: StatusDown, Detail: "No answer within 2 seconds"}
	select {
	case result = <-done:
	case <-probeCtx.Done():
	}
	return Component{
		Name:      probe.Name,
		Status:    result.Status,
		Detail:    result.Detail,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func overallStatus(components []Component) Status {
	overall := StatusOK
	for _, comp := range components {
		if StatusDown == comp.Status {
			return StatusDown
		}
		if StatusWarn == comp.Status {
			overall = StatusWarn
		}
	}
	return overall
}

func ok(detail string) Result {
	return Result{Status: StatusOK, Detail: detail}
}

func warned(detail string) Result {
	return Result{Status: StatusWarn, Detail: detail}
}

func disabled(detail string) Result {
	return Result{Status: StatusDisabled, Detail: detail}
}

func failed(err error) Result {
	return Result{Status: StatusDown, Detail: err.Error()}
}

func FileIndexProbe(ping func(ctx context.Context) error) Probe {
	return Probe{Name: "file_index", Run: func(ctx context.Context) Result {
		if err := ping(ctx); nil != err {
			return failed(err)
		}
		return ok("Reachable")
	}}
}

func LocksProbe(manager locks.Manager, instanceID string) Probe {
	return Probe{Name: "locks", Run: func(ctx context.Context) Result {
		lease, err := manager.Acquire(ctx, "health:"+instanceID)
		if nil != err {
			return failed(err)
		}
		if releaseErr := lease.Release(ctx); nil != releaseErr {
			return warned(releaseErr.Error())
		}
		return ok("Lock acquired and released")
	}}
}

func StorageProbe(name string, enabled bool, stat func(ctx context.Context) error) Probe {
	return Probe{Name: name, Run: func(ctx context.Context) Result {
		if !enabled {
			return disabled("Not in use")
		}
		if err := stat(ctx); nil != err {
			return failed(err)
		}
		return ok("Reachable")
	}}
}

func ErasureProbe(enabled, present bool) Probe {
	return Probe{Name: "erasure", Run: func(ctx context.Context) Result {
		if !enabled {
			return disabled("Split storage is off")
		}
		if !present {
			return Result{Status: StatusDown, Detail: "Split storage manager is not running"}
		}
		return ok("Split storage is running")
	}}
}

func TLSProbe(protocol, certFile string, now func() time.Time) Probe {
	return Probe{Name: "tls", Run: func(ctx context.Context) Result {
		if "http" == protocol {
			return disabled("Connections are not encrypted")
		}
		if "" == certFile {
			if "auto" == protocol {
				return disabled("Plain HTTP fallback")
			}
			return Result{Status: StatusDown, Detail: "No certificate file is set"}
		}
		certificate, err := readCertificate(certFile)
		if nil != err {
			return failed(err)
		}
		remaining := certificate.NotAfter.Sub(now())
		if 0 >= remaining {
			return Result{Status: StatusDown, Detail: "Certificate has expired"}
		}
		days := int(math.Ceil(remaining.Hours() / 24))
		detail := fmt.Sprintf("Certificate expires in %d days", days)
		if certWarnWindow > remaining {
			return warned(detail)
		}
		return ok(detail)
	}}
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if nil != err {
		return nil, err
	}
	for block, rest := pem.Decode(data); nil != block; block, rest = pem.Decode(rest) {
		if "CERTIFICATE" == block.Type {
			return x509.ParseCertificate(block.Bytes)
		}
	}
	return nil, errors.New("no certificate found in certificate file")
}

func DiskProbe(path string, space func(path string) (free, total uint64, err error)) Probe {
	return Probe{Name: "disk", Run: func(ctx context.Context) Result {
		free, total, err := space(path)
		if nil != err {
			return failed(err)
		}
		if 0 == total {
			return warned("Disk size is unknown")
		}
		ratio := float64(free) / float64(total)
		detail := fmt.Sprintf("%d%% of disk space free", int(ratio*100))
		if diskDownRatio > ratio {
			return Result{Status: StatusDown, Detail: detail}
		}
		if diskWarnRatio > ratio {
			return warned(detail)
		}
		return ok(detail)
	}}
}

func ReplicationProbe(enabled bool, recentFailures func() int) Probe {
	return Probe{Name: "replication", Run: func(ctx context.Context) Result {
		if !enabled {
			return disabled("Backup copy is off")
		}
		if failures := recentFailures(); 0 < failures {
			return warned(fmt.Sprintf("%d backup copies failed in the last hour", failures))
		}
		return ok("No recent backup failures")
	}}
}

func LinkCleanupProbe(lastRun func() (time.Time, error)) Probe {
	return Probe{Name: "link_cleanup", Run: func(ctx context.Context) Result {
		at, err := lastRun()
		if nil != err {
			return warned(err.Error())
		}
		if at.IsZero() {
			return ok("Not run yet")
		}
		return ok("Last run " + at.UTC().Format(time.RFC3339))
	}}
}
