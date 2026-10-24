package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type options struct {
	url         string
	key         string
	userKeys    string
	peers       string
	pprof       string
	scenarios   string
	group       string
	duration    time.Duration
	warmup      time.Duration
	concurrency int
	out         string
	small       int64
	large       int64
	files       int
	features    string
	timeout     time.Duration
	list        bool
	sessions    bool
}

type scenarioResult struct {
	Scenario   string      `json:"scenario"`
	Group      string      `json:"group"`
	Skipped    string      `json:"skipped,omitempty"`
	Elapsed    float64     `json:"elapsed_seconds"`
	Iterations int64       `json:"iterations"`
	StepErrors int64       `json:"step_errors"`
	FirstError string      `json:"first_error,omitempty"`
	Ops        []OpSummary `json:"ops"`
	Profiles   []string    `json:"profiles"`
}

const maxProfileSeconds = 120

func main() {
	opts := parseFlags()
	if opts.list {
		for _, name := range scenarioNames() {
			s, _ := lookupScenario(name)
			fmt.Printf("%-28s %s\n", name, s.Group)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, opts); nil != err {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var o options
	flag.StringVar(&o.url, "url", "http://127.0.0.1:8090", "callfs base URL")
	flag.StringVar(&o.key, "key", "", "admin API key (default CALLFS_LOADGEN_ADMIN_KEY)")
	flag.StringVar(&o.userKeys, "user-keys", "", "comma-separated non-admin API keys (default CALLFS_LOADGEN_USER_KEYS)")
	flag.StringVar(&o.peers, "peers", "", "comma-separated base URLs of other cluster instances")
	flag.StringVar(&o.pprof, "pprof", "http://127.0.0.1:9091", "comma-separated metrics base URLs serving /debug/pprof; empty skips profiles")
	flag.StringVar(&o.scenarios, "scenarios", "all", "comma-separated scenario names, or all")
	flag.StringVar(&o.group, "group", "", "only scenarios in this group")
	flag.DurationVar(&o.duration, "duration", 30*time.Second, "measured run time per scenario")
	flag.DurationVar(&o.warmup, "warmup", 3*time.Second, "unmeasured warm-up per scenario")
	flag.IntVar(&o.concurrency, "concurrency", 16, "parallel workers per scenario")
	flag.StringVar(&o.out, "out", "perf-results", "output directory")
	flag.Int64Var(&o.small, "small", 16<<10, "small file size in bytes")
	flag.Int64Var(&o.large, "large", 64<<20, "large file size in bytes")
	flag.IntVar(&o.files, "files", 200, "files seeded per scenario that needs a data set")
	flag.StringVar(&o.features, "features", "", "enabled server features: erasure,replication,s3,nats,cluster")
	flag.DurationVar(&o.timeout, "timeout", 5*time.Minute, "per-request timeout")
	flag.BoolVar(&o.list, "list", false, "list scenarios and exit")
	flag.BoolVar(&o.sessions, "sessions", false, "use browser sessions with CSRF instead of API keys for user clients")
	flag.Parse()
	if "" == o.key {
		o.key = os.Getenv("CALLFS_LOADGEN_ADMIN_KEY")
	}
	if "" == o.userKeys {
		o.userKeys = os.Getenv("CALLFS_LOADGEN_USER_KEYS")
	}
	return o
}

func run(ctx context.Context, opts options) error {
	if "" == opts.key {
		return errors.New("admin key is required (-key or CALLFS_LOADGEN_ADMIN_KEY)")
	}
	if 0 >= opts.duration || time.Duration(maxProfileSeconds)*time.Second < opts.duration {
		return fmt.Errorf("duration must be between 1s and %ds", maxProfileSeconds)
	}
	selected, err := selectScenarios(opts)
	if nil != err {
		return err
	}
	runID := time.Now().UTC().Format("20060102T150405Z")
	outDir := filepath.Join(opts.out, runID)
	if err := os.MkdirAll(outDir, 0o755); nil != err {
		return err
	}
	features := parseFeatures(opts.features)
	results := make([]scenarioResult, 0, len(selected))
	for _, s := range selected {
		if nil != ctx.Err() {
			break
		}
		fmt.Printf("-> %s\n", s.Name)
		result := runScenario(ctx, opts, features, runID, outDir, s)
		results = append(results, result)
		if "" != result.Skipped {
			fmt.Printf("   skipped: %s\n", result.Skipped)
			continue
		}
		fmt.Printf("   %d iterations, %d step errors\n", result.Iterations, result.StepErrors)
	}
	return writeSummary(outDir, results)
}

func selectScenarios(opts options) ([]Scenario, error) {
	names := scenarioNames()
	if "all" != opts.scenarios {
		names = strings.Split(opts.scenarios, ",")
	}
	out := make([]Scenario, 0, len(names))
	for _, name := range names {
		s, ok := lookupScenario(strings.TrimSpace(name))
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q (use -list)", name)
		}
		if "" != opts.group && opts.group != s.Group {
			continue
		}
		out = append(out, s)
	}
	if 0 == len(out) {
		return nil, errors.New("no scenarios selected")
	}
	return out, nil
}

func parseFeatures(list string) Features {
	parts := strings.Split(list, ",")
	has := func(name string) bool { return slices.Contains(parts, name) }
	return Features{
		Erasure:     has("erasure"),
		Replication: has("replication"),
		S3:          has("s3"),
		NATS:        has("nats"),
		Cluster:     has("cluster"),
	}
}

func newEnv(opts options, features Features, recorder *Recorder, root string) (*Env, error) {
	admin, err := NewClient("admin", opts.url, opts.key, recorder, opts.timeout)
	if nil != err {
		return nil, err
	}
	env := &Env{Admin: admin, Root: root, Features: features, Sizes: Sizes{Small: opts.small, Large: opts.large, Files: opts.files}}
	for i, key := range splitList(opts.userKeys) {
		client, err := NewClient(fmt.Sprintf("user%d", i), opts.url, key, recorder, opts.timeout)
		if nil != err {
			return nil, err
		}
		env.Users = append(env.Users, client)
	}
	for i, base := range splitList(opts.peers) {
		client, err := NewClient(fmt.Sprintf("peer%d", i), base, opts.key, recorder, opts.timeout)
		if nil != err {
			return nil, err
		}
		env.Peers = append(env.Peers, client)
	}
	return env, nil
}

func splitList(list string) []string {
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		if trimmed := strings.TrimSpace(part); "" != trimmed {
			out = append(out, trimmed)
		}
	}
	return out
}

func runScenario(ctx context.Context, opts options, features Features, runID, outDir string, s Scenario) scenarioResult {
	result := scenarioResult{Scenario: s.Name, Group: s.Group}
	if nil != s.Needs && !s.Needs(features) {
		result.Skipped = "server feature not enabled (see -features)"
		return result
	}
	setupRecorder := NewRecorder()
	root := "/perf-" + runID
	env, err := newEnv(opts, features, setupRecorder, root)
	if nil != err {
		result.Skipped = err.Error()
		return result
	}
	if opts.sessions {
		for _, user := range env.Users {
			if err := user.Login(ctx); nil != err {
				result.Skipped = "session login: " + err.Error()
				return result
			}
		}
	}
	if nil != s.Setup {
		if err := s.Setup(ctx, env); nil != err {
			result.Skipped = "setup: " + err.Error()
			return result
		}
	}
	if 0 < opts.warmup {
		drive(ctx, env, s, opts.concurrency, opts.warmup, NewRecorder())
	}
	measured := NewRecorder()
	dir := filepath.Join(outDir, s.Name)
	if err := os.MkdirAll(dir, 0o755); nil != err {
		result.Skipped = err.Error()
		return result
	}
	profiles := startProfiles(ctx, opts, dir)
	started := time.Now()
	iterations, stepErrors, firstErr := drive(ctx, env, s, opts.concurrency, opts.duration, measured)
	elapsed := time.Since(started)
	result.Profiles = profiles.wait(ctx, opts, dir)
	if nil != s.Teardown {
		if err := s.Teardown(context.WithoutCancel(ctx), env); nil != err && "" == firstErr {
			firstErr = "teardown: " + err.Error()
		}
	}
	result.Elapsed = elapsed.Seconds()
	result.Iterations = iterations
	result.StepErrors = stepErrors
	result.FirstError = firstErr
	result.Ops = measured.Summaries(elapsed)
	if err := writeScenarioReport(dir, result); nil != err && "" == result.FirstError {
		result.FirstError = err.Error()
	}
	return result
}

var driveEpoch atomic.Int64

func drive(ctx context.Context, env *Env, s Scenario, concurrency int, d time.Duration, recorder *Recorder) (int64, int64, string) {
	epoch := driveEpoch.Add(1)
	runCtx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	scoped := env.withRecorder(recorder)
	var iterations, stepErrors atomic.Int64
	var firstErr atomic.Value
	var wg sync.WaitGroup
	for id := 0; id < concurrency; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w := &Worker{Epoch: epoch, ID: id, Rand: rand.New(rand.NewPCG(uint64(id)+1, uint64(time.Now().UnixNano())))}
			w.Client = scoped.User(w)
			for nil == runCtx.Err() {
				err := s.Step(runCtx, scoped, w)
				w.Iter++
				if nil != runCtx.Err() {
					return
				}
				iterations.Add(1)
				if nil != err {
					stepErrors.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
				}
			}
		}(id)
	}
	wg.Wait()
	first, _ := firstErr.Load().(string)
	return iterations.Load(), stepErrors.Load(), first
}

func (e *Env) withRecorder(recorder *Recorder) *Env {
	rebind := func(c *Client) *Client {
		clone := *c
		clone.recorder = recorder
		return &clone
	}
	scoped := &Env{Admin: rebind(e.Admin), Root: e.Root, Sizes: e.Sizes, Features: e.Features}
	for _, u := range e.Users {
		scoped.Users = append(scoped.Users, rebind(u))
	}
	for _, p := range e.Peers {
		scoped.Peers = append(scoped.Peers, rebind(p))
	}
	e.shared.Range(func(k, v any) bool {
		scoped.shared.Store(k, v)
		return true
	})
	return scoped
}

type profileSet struct {
	wg    sync.WaitGroup
	mu    sync.Mutex
	files []string
}

func startProfiles(ctx context.Context, opts options, dir string) *profileSet {
	set := &profileSet{}
	seconds := int(opts.duration.Seconds())
	for i, base := range splitList(opts.pprof) {
		for _, kind := range []string{"profile", "allocs", "mutex", "block"} {
			set.wg.Add(1)
			go func(i int, base, kind string) {
				defer set.wg.Done()
				set.fetch(ctx, fmt.Sprintf("%s/debug/pprof/%s?seconds=%d", base, kind, seconds), filepath.Join(dir, fmt.Sprintf("node%d-%s.pb.gz", i, profileName(kind))))
			}(i, base, kind)
		}
	}
	return set
}

func (p *profileSet) wait(ctx context.Context, opts options, dir string) []string {
	p.wg.Wait()
	for i, base := range splitList(opts.pprof) {
		p.fetch(ctx, base+"/debug/pprof/heap", filepath.Join(dir, fmt.Sprintf("node%d-heap.pb.gz", i)))
		p.fetch(ctx, base+"/debug/pprof/goroutine", filepath.Join(dir, fmt.Sprintf("node%d-goroutine.pb.gz", i)))
	}
	slices.Sort(p.files)
	return p.files
}

func profileName(kind string) string {
	if "profile" == kind {
		return "cpu"
	}
	return kind
}

func (p *profileSet) fetch(ctx context.Context, url, path string) {
	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(maxProfileSeconds+30)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if nil != err {
		fmt.Fprintln(os.Stderr, "profile:", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if nil != err {
		fmt.Fprintln(os.Stderr, "profile:", err)
		return
	}
	defer resp.Body.Close()
	if http.StatusOK != resp.StatusCode {
		fmt.Fprintf(os.Stderr, "profile: %s returned %d (is metrics.profiling on?)\n", url, resp.StatusCode)
		return
	}
	file, err := os.Create(path)
	if nil != err {
		fmt.Fprintln(os.Stderr, "profile:", err)
		return
	}
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if nil != copyErr || nil != closeErr {
		fmt.Fprintln(os.Stderr, "profile:", errors.Join(copyErr, closeErr))
		return
	}
	p.mu.Lock()
	p.files = append(p.files, filepath.Base(path))
	p.mu.Unlock()
}

func writeScenarioReport(dir string, result scenarioResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if nil != err {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644); nil != err {
		return err
	}
	file, err := os.Create(filepath.Join(dir, "report.txt"))
	if nil != err {
		return err
	}
	writeErr := writeTable(file, fmt.Sprintf("%s  %.0fs  %d iterations  %d step errors", result.Scenario, result.Elapsed, result.Iterations, result.StepErrors), result.Ops)
	if nil == writeErr && "" != result.FirstError {
		_, writeErr = fmt.Fprintf(file, "\nfirst step error: %s\n", result.FirstError)
	}
	return errors.Join(writeErr, file.Close())
}

func writeSummary(outDir string, results []scenarioResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if nil != err {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "summary.json"), data, 0o644); nil != err {
		return err
	}
	file, err := os.Create(filepath.Join(outDir, "summary.txt"))
	if nil != err {
		return err
	}
	var writeErr error
	all := []OpSummary{}
	for _, r := range results {
		if "" != r.Skipped {
			_, writeErr = fmt.Fprintf(file, "skipped %-26s %s\n", r.Scenario, r.Skipped)
			if nil != writeErr {
				break
			}
			continue
		}
		for _, op := range r.Ops {
			op.Op = r.Scenario + " " + op.Op
			all = append(all, op)
		}
	}
	if nil == writeErr {
		slices.SortFunc(all, func(a, b OpSummary) int {
			if a.P99Ms > b.P99Ms {
				return -1
			}
			if a.P99Ms < b.P99Ms {
				return 1
			}
			return 0
		})
		writeErr = writeTable(file, "all operations, slowest p99 first", all)
	}
	fmt.Println("results:", outDir)
	return errors.Join(writeErr, file.Close())
}
