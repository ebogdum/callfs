package health

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata/natsmeta"
)

func componentNamed(t *testing.T, report Report, name string) Component {
	t.Helper()
	for _, component := range report.Components {
		if name == component.Name {
			return component
		}
	}
	t.Fatalf("component %s missing", name)
	return Component{}
}

func TestReportAggregatesStatusAndKeepsSince(t *testing.T) {
	start := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	clock := start
	down := false
	checker := NewChecker([]Probe{
		{Name: "file_index", Run: func(ctx context.Context) Result { return ok("Reachable") }},
		{Name: "disk", Run: func(ctx context.Context) Result {
			if down {
				return Result{Status: StatusDown, Detail: "Disk full"}
			}
			return warned("8% of disk space free")
		}},
		{Name: "tls", Run: func(ctx context.Context) Result { return disabled("Connections are not encrypted") }},
	})
	checker.clock = func() time.Time { return clock }

	report := checker.Report(context.Background(), true)
	if StatusWarn != report.Status {
		t.Fatalf("overall status = %s, want warn", report.Status)
	}

	clock = start.Add(time.Minute)
	down = true
	report = checker.Report(context.Background(), true)
	if StatusDown != report.Status {
		t.Fatalf("overall status = %s, want down", report.Status)
	}
	if disk := componentNamed(t, report, "disk"); StatusDown != disk.Status || !disk.Since.Equal(clock) {
		t.Fatalf("disk = %+v, want down since %s", disk, clock)
	}
	if fileIndex := componentNamed(t, report, "file_index"); !fileIndex.Since.Equal(start) {
		t.Fatalf("file_index since = %s, want %s", fileIndex.Since, start)
	}
}

func TestReportUsesCacheWithinTTL(t *testing.T) {
	start := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	clock := start
	var runs atomic.Int32
	checker := NewChecker([]Probe{
		{Name: "locks", Run: func(ctx context.Context) Result {
			runs.Add(1)
			return ok("Lock acquired and released")
		}},
	})
	checker.clock = func() time.Time { return clock }

	checker.Report(context.Background(), false)
	clock = start.Add(4 * time.Second)
	checker.Report(context.Background(), false)
	if 1 != runs.Load() {
		t.Fatalf("probe runs = %d, want 1 within TTL", runs.Load())
	}

	clock = start.Add(6 * time.Second)
	checker.Report(context.Background(), false)
	if 2 != runs.Load() {
		t.Fatalf("probe runs = %d, want 2 after TTL", runs.Load())
	}

	checker.Report(context.Background(), true)
	if 3 != runs.Load() {
		t.Fatalf("probe runs = %d, want 3 when fresh", runs.Load())
	}
}

func TestSlowProbeIsReportedDown(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	checker := NewChecker([]Probe{
		{Name: "metadata_log", Run: func(ctx context.Context) Result {
			<-release
			return ok("late")
		}},
	})

	report := checker.Report(context.Background(), true)
	if slow := componentNamed(t, report, "metadata_log"); StatusDown != slow.Status {
		t.Fatalf("metadata_log status = %s, want down", slow.Status)
	}
}

func TestTLSProbeReportsExpiringCertificate(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	certPath := filepath.Join(t.TempDir(), "server.pem")
	writeSelfSignedCert(t, certPath, now.Add(-time.Hour), now.Add(10*24*time.Hour))

	result := TLSProbe("https", certPath, func() time.Time { return now }).Run(context.Background())
	if StatusWarn != result.Status || "Certificate expires in 10 days" != result.Detail {
		t.Fatalf("tls result = %+v, want warn with 10 days", result)
	}
}

func writeSelfSignedCert(t *testing.T, path string, notBefore, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if nil != err {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "callfs.test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if nil != err {
		t.Fatalf("create certificate: %v", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); nil != err {
		t.Fatalf("write certificate: %v", err)
	}
}

func TestInformationalComponentDoesNotFailOverall(t *testing.T) {
	checker := NewChecker([]Probe{
		{Name: "locks", Run: func(ctx context.Context) Result { return ok("fine") }},
		{Name: "storage.internal_proxy", Informational: true, Run: func(ctx context.Context) Result {
			return Result{Status: StatusDown, Detail: "peer down"}
		}},
	})

	report := checker.Report(context.Background(), true)
	if StatusOK != report.Status {
		t.Fatalf("overall = %s, want ok", report.Status)
	}
	peer := componentNamed(t, report, "storage.internal_proxy")
	if StatusDown != peer.Status || !peer.Informational {
		t.Fatalf("peer = %+v, want down and informational", peer)
	}
}

type fakeMetadataLog struct {
	info     natsmeta.LogInfo
	readyErr error
}

func (f fakeMetadataLog) LogInfo(ctx context.Context) (natsmeta.LogInfo, error) { return f.info, nil }

func (f fakeMetadataLog) Ready() error { return f.readyErr }

func TestMetadataLogProbe(t *testing.T) {
	cases := []struct {
		name string
		log  fakeMetadataLog
		want Status
	}{
		{"caught up", fakeMetadataLog{info: natsmeta.LogInfo{LastSeq: 10, AppliedSeq: 10}}, StatusOK},
		{"small lag", fakeMetadataLog{info: natsmeta.LogInfo{LastSeq: 500, AppliedSeq: 10}}, StatusWarn},
		{"large lag", fakeMetadataLog{info: natsmeta.LogInfo{LastSeq: 90000, AppliedSeq: 10}}, StatusDown},
		{"halted", fakeMetadataLog{readyErr: errors.New("halted")}, StatusDown},
		{"one of three offline", fakeMetadataLog{info: natsmeta.LogInfo{Replicas: []natsmeta.ReplicaInfo{{}, {}, {Offline: true}}}}, StatusWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := MetadataLogProbe(true, tc.log).Run(context.Background())
			if tc.want != result.Status {
				t.Fatalf("status = %s (%s), want %s", result.Status, result.Detail, tc.want)
			}
		})
	}
}

func TestNATSProbe(t *testing.T) {
	if StatusDisabled != NATSProbe(false, nil).Run(context.Background()).Status {
		t.Fatal("disabled probe must report disabled")
	}
	if StatusDown != NATSProbe(true, func() bool { return false }).Run(context.Background()).Status {
		t.Fatal("disconnected probe must report down")
	}
	if StatusOK != NATSProbe(true, func() bool { return true }).Run(context.Background()).Status {
		t.Fatal("connected probe must report ok")
	}
}
