package findings

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ebogdum/callfs/v2/config"
)

var (
	ErrUnknownFinding   = errors.New("unknown finding")
	ErrFindingNotActive = errors.New("finding is not active")
	ErrInvalidMute      = errors.New("invalid mute")
	ErrNoConfig         = errors.New("configuration is unavailable")
	ErrNoMuteStore      = errors.New("mute store is unavailable")
	ErrInvalidIgnore    = errors.New("invalid ignore")
	ErrNoIgnoreStore    = errors.New("ignore store is unavailable")
)

const (
	minMuteDuration      = time.Minute
	maxMuteDuration      = 7 * 24 * time.Hour
	maxMuteReasonRunes   = 200
	maxIgnoreReasonRunes = 200
)

type Entity struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Href string `json:"href"`
}

type Fix struct {
	Label      string  `json:"label"`
	Route      string  `json:"route"`
	SettingKey *string `json:"setting_key"`
	Command    *string `json:"command"`
}

type Mute struct {
	Until  time.Time `json:"until"`
	By     string    `json:"by"`
	Reason string    `json:"reason"`
}

type Ignore struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Reason string    `json:"reason"`
}

type Finding struct {
	ID       string    `json:"id"`
	Severity string    `json:"severity"`
	Area     string    `json:"area"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	Affected []Entity  `json:"affected"`
	Since    time.Time `json:"since"`
	Fix      Fix       `json:"fix"`
	Muted    *Mute     `json:"muted"`
	Ignored  *Ignore   `json:"ignored"`
}

type Counts struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

func (c *Counts) add(severity string) {
	switch severity {
	case SeverityCritical:
		c.Critical++
	case SeverityWarning:
		c.Warning++
	case SeverityInfo:
		c.Info++
	}
}

type DownInstance struct {
	ID        string
	DownSince time.Time
}

type Facts struct {
	CertNotAfter            time.Time
	CertUnreadable          bool
	KeyWorldReadable        bool
	AutoFellBack            bool
	InstancesDown           []DownInstance
	DiskFreeBytes           uint64
	DiskTotalBytes          uint64
	ErasureMaxOnOneInstance int
	ErasureReduced          int
	ErasureCritical         int
	ReplicationFailures1h   int
	StagingBytes            uint64
	StagingCount            int
	FilesNoOwner            int
	FilesNoETag             int
	MigrationsDirty         bool
	TopClientShare          float64
	LinkOpens1h             int
	LinkFailedOpens1h       int
	AuthFailures10m         int
	PendingRestart          int
	NATSUnreachable         bool
	MetadataReplicas        int
	MetadataOffline         int
	MetadataLogLag          uint64
	LastSnapshotAt          time.Time
}

type Source func(ctx context.Context) (Facts, error)

type MuteStore interface {
	LoadMutes(ctx context.Context) (map[string]Mute, error)
	SaveMutes(ctx context.Context, mutes map[string]Mute) error
}

type IgnoreStore interface {
	LoadIgnores(ctx context.Context) (map[string]Ignore, error)
	SaveIgnores(ctx context.Context, ignores map[string]Ignore) error
}

type Engine struct {
	current       func() *config.AppConfig
	source        Source
	store         MuteStore
	ignoreStore   IgnoreStore
	now           func() time.Time
	mu            sync.Mutex
	loaded        bool
	mutes         map[string]Mute
	ignoresLoaded bool
	ignores       map[string]Ignore
	firstSeen     map[string]time.Time
}

func NewEngine(current func() *config.AppConfig, source Source, store MuteStore, ignoreStore IgnoreStore) *Engine {
	return &Engine{
		current:     current,
		source:      source,
		store:       store,
		ignoreStore: ignoreStore,
		now:         time.Now,
		firstSeen:   make(map[string]time.Time),
	}
}

func (e *Engine) Evaluate(ctx context.Context) ([]Finding, error) {
	cfg := e.current()
	if nil == cfg {
		return nil, ErrNoConfig
	}
	facts, err := e.source(ctx)
	if nil != err {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadLocked(ctx); nil != err {
		return nil, err
	}
	if err := e.loadIgnoresLocked(ctx); nil != err {
		return nil, err
	}
	now := e.now()
	in := input{cfg: cfg, facts: facts, now: now}
	findings := make([]Finding, 0, len(catalog))
	active := make(map[string]bool, len(catalog))
	for _, entry := range catalog {
		h := entry.check(in)
		if nil == h {
			continue
		}
		finding := entry.finding(h)
		since, known := e.firstSeen[finding.ID]
		if !known {
			since = now
		}
		e.firstSeen[finding.ID] = since
		finding.Since = since
		active[finding.ID] = true
		if mute, muted := e.mutes[finding.ID]; muted && now.Before(mute.Until) {
			finding.Muted = &mute
		}
		if ignore, ignored := e.ignores[finding.ID]; ignored {
			finding.Ignored = &ignore
		}
		findings = append(findings, finding)
	}
	for id := range e.firstSeen {
		if !active[id] {
			delete(e.firstSeen, id)
		}
	}
	slices.SortFunc(findings, func(a, b Finding) int {
		if rank := cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)); 0 != rank {
			return rank
		}
		return strings.Compare(a.ID, b.ID)
	})
	return findings, nil
}

func (e *Engine) Mute(ctx context.Context, id, by string, duration time.Duration, reason string) (Finding, error) {
	if _, known := findRule(id); !known {
		return Finding{}, ErrUnknownFinding
	}
	if "" == by || minMuteDuration > duration || maxMuteDuration < duration || maxMuteReasonRunes < utf8.RuneCountInString(reason) {
		return Finding{}, ErrInvalidMute
	}
	err := e.update(ctx, func(mutes map[string]Mute) {
		mutes[id] = Mute{Until: e.now().Add(duration), By: by, Reason: reason}
	})
	if nil != err {
		return Finding{}, err
	}
	return e.lookup(ctx, id)
}

func (e *Engine) Unmute(ctx context.Context, id string) (Finding, error) {
	if _, known := findRule(id); !known {
		return Finding{}, ErrUnknownFinding
	}
	err := e.update(ctx, func(mutes map[string]Mute) {
		delete(mutes, id)
	})
	if nil != err {
		return Finding{}, err
	}
	return e.lookup(ctx, id)
}

func (e *Engine) Ignore(ctx context.Context, id, by, reason string) (Finding, error) {
	if _, known := findRule(id); !known {
		return Finding{}, ErrUnknownFinding
	}
	if "" == by || maxIgnoreReasonRunes < utf8.RuneCountInString(reason) {
		return Finding{}, ErrInvalidIgnore
	}
	if _, err := e.lookup(ctx, id); nil != err {
		return Finding{}, err
	}
	err := e.updateIgnores(ctx, func(ignores map[string]Ignore) {
		ignores[id] = Ignore{At: e.now(), By: by, Reason: reason}
	})
	if nil != err {
		return Finding{}, err
	}
	return e.lookup(ctx, id)
}

func (e *Engine) Unignore(ctx context.Context, id string) error {
	if _, known := findRule(id); !known {
		return ErrUnknownFinding
	}
	return e.updateIgnores(ctx, func(ignores map[string]Ignore) {
		delete(ignores, id)
	})
}

func (e *Engine) updateIgnores(ctx context.Context, apply func(ignores map[string]Ignore)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadIgnoresLocked(ctx); nil != err {
		return err
	}
	next := maps.Clone(e.ignores)
	apply(next)
	if err := e.ignoreStore.SaveIgnores(ctx, next); nil != err {
		return err
	}
	e.ignores = next
	return nil
}

func (e *Engine) loadIgnoresLocked(ctx context.Context) error {
	if e.ignoresLoaded {
		return nil
	}
	if nil == e.ignoreStore {
		return ErrNoIgnoreStore
	}
	ignores, err := e.ignoreStore.LoadIgnores(ctx)
	if nil != err {
		return err
	}
	if nil == ignores {
		ignores = make(map[string]Ignore)
	}
	e.ignores = ignores
	e.ignoresLoaded = true
	return nil
}

func (e *Engine) lookup(ctx context.Context, id string) (Finding, error) {
	findings, err := e.Evaluate(ctx)
	if nil != err {
		return Finding{}, err
	}
	for _, finding := range findings {
		if id == finding.ID {
			return finding, nil
		}
	}
	return Finding{}, ErrFindingNotActive
}

func (e *Engine) update(ctx context.Context, apply func(mutes map[string]Mute)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadLocked(ctx); nil != err {
		return err
	}
	next := maps.Clone(e.mutes)
	apply(next)
	pruneExpired(next, e.now())
	if err := e.store.SaveMutes(ctx, next); nil != err {
		return err
	}
	e.mutes = next
	return nil
}

func (e *Engine) loadLocked(ctx context.Context) error {
	if e.loaded {
		return nil
	}
	if nil == e.store {
		return ErrNoMuteStore
	}
	mutes, err := e.store.LoadMutes(ctx)
	if nil != err {
		return err
	}
	if nil == mutes {
		mutes = make(map[string]Mute)
	}
	pruneExpired(mutes, e.now())
	e.mutes = mutes
	e.loaded = true
	return nil
}

func pruneExpired(mutes map[string]Mute, now time.Time) {
	for id, mute := range mutes {
		if !now.Before(mute.Until) {
			delete(mutes, id)
		}
	}
}

func severityRank(severity string) int {
	switch severity {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	}
	return 2
}

type Selection struct {
	Items   []Finding
	Ignored []Finding
	Counts  Counts
}

func Select(findings []Finding, area string, includeMuted bool) Selection {
	selected := make([]Finding, 0, len(findings))
	ignored := make([]Finding, 0)
	var counts Counts
	for _, finding := range findings {
		if "" != area && area != finding.Area {
			continue
		}
		if nil != finding.Ignored {
			ignored = append(ignored, finding)
			continue
		}
		if nil != finding.Muted && !includeMuted {
			continue
		}
		selected = append(selected, finding)
		if nil == finding.Muted {
			counts.add(finding.Severity)
		}
	}
	return Selection{Items: selected, Ignored: ignored, Counts: counts}
}

func ValidArea(area string) bool {
	switch area {
	case AreaSecurity, AreaStorage, AreaCluster, AreaConfig, AreaRuntime:
		return true
	}
	return false
}
