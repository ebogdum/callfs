package findings

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata/natsmeta"
)

type memoryMutes struct {
	mutes       map[string]Mute
	saves       int
	ignores     map[string]Ignore
	ignoreSaves int
}

func (m *memoryMutes) LoadIgnores(ctx context.Context) (map[string]Ignore, error) {
	return maps.Clone(m.ignores), nil
}

func (m *memoryMutes) SaveIgnores(ctx context.Context, ignores map[string]Ignore) error {
	m.ignores = maps.Clone(ignores)
	m.ignoreSaves++
	return nil
}

func (m *memoryMutes) LoadMutes(ctx context.Context) (map[string]Mute, error) {
	return maps.Clone(m.mutes), nil
}

func (m *memoryMutes) SaveMutes(ctx context.Context, mutes map[string]Mute) error {
	m.mutes = maps.Clone(mutes)
	m.saves++
	return nil
}

func healthyConfig() *config.AppConfig {
	cfg := &config.AppConfig{}
	cfg.Server.Protocol = "https"
	cfg.Server.ReadTimeout = 10 * time.Minute
	cfg.Server.WriteTimeout = 10 * time.Minute
	cfg.Auth.InternalProxySecret = strings.Repeat("a", 40)
	cfg.Auth.SingleUseLinkSecret = strings.Repeat("b", 40)
	cfg.Auth.AdminUsers = []string{"ops"}
	cfg.Backend.S3ACL = "private"
	cfg.Backend.S3ServerSideEncryption = "AES256"
	cfg.DLM.Type = "redis"
	cfg.DLM.RedisPassword = "pw"
	cfg.InstanceDiscovery.InstanceID = "node-a"
	cfg.MetadataStore.Type = "sqlite"
	return cfg
}

func newTestEngine(cfg *config.AppConfig, facts *Facts, store *memoryMutes, now time.Time) *Engine {
	engine := NewEngine(
		func() *config.AppConfig { return cfg },
		func(ctx context.Context) (Facts, error) { return *facts, nil },
		store,
		store,
	)
	engine.now = func() time.Time { return now }
	return engine
}

func findByID(findings []Finding, id string) (Finding, bool) {
	for _, finding := range findings {
		if id == finding.ID {
			return finding, true
		}
	}
	return Finding{}, false
}

var testNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func TestEvaluateHealthyConfigHasNoFindings(t *testing.T) {
	engine := newTestEngine(healthyConfig(), &Facts{}, &memoryMutes{}, testNow)
	findings, err := engine.Evaluate(context.Background())
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	if 0 != len(findings) {
		t.Fatalf("findings = %d, want 0", len(findings))
	}
}

func TestEvaluatePlainHTTPIsCritical(t *testing.T) {
	cfg := healthyConfig()
	cfg.Server.Protocol = "http"
	engine := newTestEngine(cfg, &Facts{}, &memoryMutes{}, testNow)
	findings, err := engine.Evaluate(context.Background())
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	finding, found := findByID(findings, "tls.plain_http")
	if !found {
		t.Fatalf("tls.plain_http missing")
	}
	if SeverityCritical != finding.Severity || "Connections are not encrypted" != finding.Title {
		t.Fatalf("finding = %+v", finding)
	}
	if nil == finding.Fix.SettingKey || "server.protocol" != *finding.Fix.SettingKey {
		t.Fatalf("fix setting key = %v, want server.protocol", finding.Fix.SettingKey)
	}
}

func TestMuteHidesFindingUntilExpiry(t *testing.T) {
	store := &memoryMutes{}
	facts := &Facts{DiskFreeBytes: 4, DiskTotalBytes: 100}
	engine := newTestEngine(healthyConfig(), facts, store, testNow)

	finding, err := engine.Mute(context.Background(), "disk.low", "root", time.Hour, "Planned cleanup")
	if nil != err {
		t.Fatalf("mute: %v", err)
	}
	if nil == finding.Muted || "root" != finding.Muted.By || !finding.Muted.Until.Equal(testNow.Add(time.Hour)) {
		t.Fatalf("muted = %+v", finding.Muted)
	}
	if 1 != store.saves {
		t.Fatalf("saves = %d, want 1", store.saves)
	}

	findings, err := engine.Evaluate(context.Background())
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	if active := Select(findings, "", false); 0 != len(active.Items) || 0 != active.Counts.Critical+active.Counts.Warning+active.Counts.Info {
		t.Fatalf("active = %d, counts = %+v, want none", len(active.Items), active.Counts)
	}
	if all := Select(findings, "", true); 1 != len(all.Items) {
		t.Fatalf("include_muted = %d, want 1", len(all.Items))
	}

	engine.now = func() time.Time { return testNow.Add(2 * time.Hour) }
	findings, err = engine.Evaluate(context.Background())
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	if finding, _ := findByID(findings, "disk.low"); nil != finding.Muted {
		t.Fatalf("mute still active after expiry")
	}
}

func TestSinceKeepsFirstSeenTime(t *testing.T) {
	facts := &Facts{DiskFreeBytes: 4, DiskTotalBytes: 100}
	clock := testNow
	engine := newTestEngine(healthyConfig(), facts, &memoryMutes{}, clock)
	engine.now = func() time.Time { return clock }
	ctx := context.Background()

	findings, _ := engine.Evaluate(ctx)
	if finding, _ := findByID(findings, "disk.low"); !finding.Since.Equal(testNow) {
		t.Fatalf("since = %s, want %s", finding.Since, testNow)
	}

	clock = testNow.Add(time.Hour)
	findings, _ = engine.Evaluate(ctx)
	if finding, _ := findByID(findings, "disk.low"); !finding.Since.Equal(testNow) {
		t.Fatalf("since moved to %s", finding.Since)
	}

	facts.DiskFreeBytes = 50
	clock = testNow.Add(2 * time.Hour)
	engine.Evaluate(ctx)

	facts.DiskFreeBytes = 4
	clock = testNow.Add(3 * time.Hour)
	findings, _ = engine.Evaluate(ctx)
	if finding, _ := findByID(findings, "disk.low"); !finding.Since.Equal(clock) {
		t.Fatalf("since after recurrence = %s, want %s", finding.Since, clock)
	}
}

func TestMuteRejectsInvalidInput(t *testing.T) {
	engine := newTestEngine(healthyConfig(), &Facts{}, &memoryMutes{}, testNow)
	ctx := context.Background()
	longReason := strings.Repeat("x", maxMuteReasonRunes+1)
	cases := []struct {
		name     string
		id       string
		by       string
		duration time.Duration
		reason   string
		want     error
	}{
		{"unknown id", "nope.id", "root", time.Hour, "", ErrUnknownFinding},
		{"zero duration", "admin.none", "root", 0, "", ErrInvalidMute},
		{"too long", "admin.none", "root", 8 * 24 * time.Hour, "", ErrInvalidMute},
		{"no actor", "admin.none", "", time.Hour, "", ErrInvalidMute},
		{"long reason", "admin.none", "root", time.Hour, longReason, ErrInvalidMute},
		{"not active", "disk.low", "root", time.Hour, "", ErrFindingNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Mute(ctx, tc.id, tc.by, tc.duration, tc.reason)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIgnorePersistsAndLeavesCounts(t *testing.T) {
	store := &memoryMutes{}
	cfg := healthyConfig()
	cfg.Server.Protocol = "http"
	engine := newTestEngine(cfg, &Facts{}, store, testNow)
	ctx := context.Background()

	finding, err := engine.Ignore(ctx, "tls.plain_http", "root", "Internal network")
	if nil != err {
		t.Fatalf("ignore: %v", err)
	}
	if nil == finding.Ignored || "root" != finding.Ignored.By || "Internal network" != finding.Ignored.Reason || !finding.Ignored.At.Equal(testNow) {
		t.Fatalf("ignored = %+v", finding.Ignored)
	}
	if 1 != store.ignoreSaves {
		t.Fatalf("ignore saves = %d, want 1", store.ignoreSaves)
	}
	if _, kept := store.ignores["tls.plain_http"]; !kept {
		t.Fatalf("ignore not persisted: %+v", store.ignores)
	}

	restarted := newTestEngine(cfg, &Facts{}, store, testNow.Add(30*24*time.Hour))
	findings, err := restarted.Evaluate(ctx)
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	for _, includeMuted := range []bool{false, true} {
		selection := Select(findings, "", includeMuted)
		if _, listed := findByID(selection.Items, "tls.plain_http"); listed {
			t.Fatalf("include_muted=%v: ignored finding listed in items", includeMuted)
		}
		if _, listed := findByID(selection.Ignored, "tls.plain_http"); !listed {
			t.Fatalf("include_muted=%v: ignored finding missing from ignored", includeMuted)
		}
		if 0 != selection.Counts.Critical {
			t.Fatalf("include_muted=%v: critical = %d, want 0", includeMuted, selection.Counts.Critical)
		}
	}
	if selection := Select(findings, AreaStorage, false); 0 != len(selection.Ignored) {
		t.Fatalf("area filter kept %d ignored, want 0", len(selection.Ignored))
	}

	if err := restarted.Unignore(ctx, "tls.plain_http"); nil != err {
		t.Fatalf("unignore: %v", err)
	}
	if 0 != len(store.ignores) {
		t.Fatalf("ignores after unignore = %+v", store.ignores)
	}
	findings, err = restarted.Evaluate(ctx)
	if nil != err {
		t.Fatalf("evaluate: %v", err)
	}
	selection := Select(findings, "", false)
	if 0 != len(selection.Ignored) || 1 > selection.Counts.Critical {
		t.Fatalf("after unignore ignored = %d, critical = %d", len(selection.Ignored), selection.Counts.Critical)
	}
}

func TestIgnoreRejectsInvalidInput(t *testing.T) {
	store := &memoryMutes{}
	cfg := healthyConfig()
	cfg.Server.Protocol = "http"
	engine := newTestEngine(cfg, &Facts{}, store, testNow)
	ctx := context.Background()
	cases := []struct {
		name   string
		id     string
		by     string
		reason string
		want   error
	}{
		{"unknown id", "nope.id", "root", "", ErrUnknownFinding},
		{"no actor", "tls.plain_http", "", "", ErrInvalidIgnore},
		{"long reason", "tls.plain_http", "root", strings.Repeat("x", maxIgnoreReasonRunes+1), ErrInvalidIgnore},
		{"not active", "disk.low", "root", "", ErrFindingNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Ignore(ctx, tc.id, tc.by, tc.reason)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if 0 != store.ignoreSaves {
		t.Fatalf("ignore saves = %d, want 0", store.ignoreSaves)
	}
	if err := engine.Unignore(ctx, "nope.id"); !errors.Is(err, ErrUnknownFinding) {
		t.Fatalf("unignore unknown = %v, want %v", err, ErrUnknownFinding)
	}
}

func TestEvaluateWithoutIgnoreStoreFails(t *testing.T) {
	cfg := healthyConfig()
	engine := NewEngine(
		func() *config.AppConfig { return cfg },
		func(ctx context.Context) (Facts, error) { return Facts{}, nil },
		&memoryMutes{},
		nil,
	)
	if _, err := engine.Evaluate(context.Background()); !errors.Is(err, ErrNoIgnoreStore) {
		t.Fatalf("err = %v, want %v", err, ErrNoIgnoreStore)
	}
}

func TestCatalogTextLimits(t *testing.T) {
	for _, entry := range catalog {
		if words := len(strings.Fields(entry.title)); 8 < words {
			t.Errorf("%s: title has %d words", entry.id, words)
		}
		sentences := strings.Split(strings.TrimSpace(entry.body), ". ")
		if 2 < len(sentences) {
			t.Errorf("%s: body has %d sentences", entry.id, len(sentences))
		}
		for _, sentence := range sentences {
			if words := len(strings.Fields(sentence)); 20 < words {
				t.Errorf("%s: sentence has %d words", entry.id, words)
			}
		}
	}
}

func natsConfig() *config.AppConfig {
	cfg := healthyConfig()
	cfg.NATS.Enabled = true
	cfg.NATS.Metadata.SnapshotInterval = time.Hour
	return cfg
}

func TestNATSFindings(t *testing.T) {
	cases := []struct {
		name  string
		facts Facts
		want  string
	}{
		{"not connected", Facts{NATSUnreachable: true}, "nats.not_connected"},
		{"no quorum", Facts{MetadataReplicas: 3, MetadataOffline: 2}, "nats.no_quorum"},
		{"replica offline", Facts{MetadataReplicas: 3, MetadataOffline: 1}, "nats.replica_offline"},
		{"log lag", Facts{MetadataLogLag: 5000}, "nats.log_lag"},
		{"snapshot stale", Facts{LastSnapshotAt: testNow.Add(-4 * time.Hour)}, "nats.snapshot_stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := tc.facts
			engine := newTestEngine(natsConfig(), &facts, &memoryMutes{}, testNow)
			result, err := engine.Evaluate(context.Background())
			if nil != err {
				t.Fatal(err)
			}
			if _, found := findByID(result, tc.want); !found {
				t.Fatalf("finding %s missing", tc.want)
			}
		})
	}
}

func TestNATSFindingsQuietWhenHealthy(t *testing.T) {
	facts := &Facts{MetadataReplicas: 3, LastSnapshotAt: testNow.Add(-time.Hour), MetadataLogLag: 5}
	engine := newTestEngine(natsConfig(), facts, &memoryMutes{}, testNow)
	result, err := engine.Evaluate(context.Background())
	if nil != err {
		t.Fatal(err)
	}
	if 0 != len(result) {
		t.Fatalf("findings = %+v, want none", result)
	}
}

type fakeMetadataLog struct {
	info natsmeta.LogInfo
	err  error
}

func (f fakeMetadataLog) LogInfo(ctx context.Context) (natsmeta.LogInfo, error) {
	return f.info, f.err
}

func TestCollectNATSFacts(t *testing.T) {
	var facts Facts
	CollectNATSFacts(context.Background(), fakeMetadataLog{info: natsmeta.LogInfo{
		LastSeq:      100,
		AppliedSeq:   40,
		Replicas:     []natsmeta.ReplicaInfo{{}, {Offline: true}, {}},
		LastSnapshot: &natsmeta.SnapshotInfo{CreatedAt: testNow},
	}}, &facts)
	if 3 != facts.MetadataReplicas || 1 != facts.MetadataOffline || 60 != facts.MetadataLogLag || !testNow.Equal(facts.LastSnapshotAt) {
		t.Fatalf("facts = %+v", facts)
	}
	var unreachable Facts
	CollectNATSFacts(context.Background(), fakeMetadataLog{err: errors.New("down")}, &unreachable)
	if !unreachable.NATSUnreachable {
		t.Fatal("log error must mark NATS unreachable")
	}
}
