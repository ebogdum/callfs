package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type adminRead struct {
	op     string
	path   func(w *Worker) string
	expect []int
	needs  func(Features) bool
}

type metricFamily struct {
	name    string
	measure string
}

var metricFamilies = []metricFamily{
	{"callfs_active_locks", "value"},
	{"callfs_auth_failures_total", "rate"},
	{"callfs_backend_op_duration_seconds", "p95"},
	{"callfs_backend_ops_total", "rate"},
	{"callfs_cache_entries", "value"},
	{"callfs_cache_evictions_total", "rate"},
	{"callfs_cache_lookups_total", "rate"},
	{"callfs_errors_total", "total"},
	{"callfs_file_operations_total", "rate"},
	{"callfs_http_request_duration_seconds", "p99"},
	{"callfs_http_requests_total", "rate"},
	{"callfs_jobs_total", "total"},
	{"callfs_lock_operation_duration_seconds", "avg"},
	{"callfs_lock_operations_total", "rate"},
	{"callfs_metadata_db_queries_total", "rate"},
	{"callfs_metadata_db_query_duration_seconds", "p50"},
	{"callfs_rate_limit_rejections_total", "total"},
	{"callfs_transfers_active", "value"},
	{"callfs_usage_bytes", "value"},
	{"callfs_usage_files", "value"},
}

var metricSteps = []string{"5", "15", "60"}

var configExportFormats = []string{"yaml", "json", "env"}

var cacheOrDisabled = []int{http.StatusOK, http.StatusNotFound}

func fixedPath(path string) func(*Worker) string {
	return func(*Worker) string { return path }
}

func adminReadTable() []adminRead {
	clustered := func(f Features) bool { return f.Cluster }
	return []adminRead{
		{op: "admin.status", path: fixedPath("/v1/admin/status")},
		{op: "admin.migrations", path: fixedPath("/v1/admin/migrations")},
		{op: "admin.health", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/health"
			}
			return "/v1/admin/health?fresh=true"
		}},
		{op: "admin.findings", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/findings"
			}
			return "/v1/admin/findings?include_muted=true"
		}},
		{op: "admin.tls", path: fixedPath("/v1/admin/tls")},
		{op: "admin.config", path: fixedPath("/v1/admin/config")},
		{op: "admin.config_schema", path: fixedPath("/v1/admin/config/schema")},
		{op: "admin.config_export", path: func(w *Worker) string {
			return "/v1/admin/config/export?format=" + configExportFormats[w.Pick(len(configExportFormats))]
		}},
		{op: "admin.cluster", path: fixedPath("/v1/admin/cluster")},
		{op: "admin.cluster_snapshots", path: fixedPath("/v1/admin/cluster/snapshots"), needs: clustered},
		{op: "admin.cluster_state_stats", path: fixedPath("/v1/admin/cluster/state-stats"), needs: clustered},
		{op: "admin.instances", path: fixedPath("/v1/admin/instances")},
		{op: "admin.backends", path: fixedPath("/v1/admin/backends")},
		{op: "admin.erasure", path: fixedPath("/v1/admin/erasure")},
		{op: "admin.replication", path: fixedPath("/v1/admin/replication")},
		{op: "admin.staging", path: fixedPath("/v1/admin/staging")},
		{op: "admin.hygiene", path: fixedPath("/v1/admin/hygiene")},
		{op: "admin.locks", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/locks"
			}
			return "/v1/admin/locks?limit=500"
		}, expect: cacheOrDisabled},
		{op: "admin.cache", path: fixedPath("/v1/admin/cache")},
		{op: "admin.ratelimits", path: fixedPath("/v1/admin/ratelimits"), expect: cacheOrDisabled},
		{op: "admin.users", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/users"
			}
			return "/v1/admin/users?limit=1000"
		}},
		{op: "admin.owners", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/owners"
			}
			return "/v1/admin/owners?limit=1000"
		}},
		{op: "admin.auth_failures", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/auth/failures"
			}
			return "/v1/admin/auth/failures?limit=1000"
		}},
		{op: "admin.metrics", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/metrics"
			}
			return "/v1/admin/metrics?names=" + url.QueryEscape(metricFamilies[w.Pick(len(metricFamilies))].name)
		}},
		{op: "admin.metrics_series", path: func(w *Worker) string {
			family := metricFamilies[w.Pick(len(metricFamilies))]
			return fmt.Sprintf("/v1/admin/metrics/series?name=%s&measure=%s&step_s=%s", url.QueryEscape(family.name), family.measure, metricSteps[w.Pick(len(metricSteps))])
		}},
		{op: "admin.logs", path: func(w *Worker) string {
			switch w.Pick(3) {
			case 0:
				return "/v1/admin/logs?limit=200"
			case 1:
				return "/v1/admin/logs?limit=1000&level=warn"
			}
			return "/v1/admin/logs?limit=500&q=request"
		}},
		{op: "admin.audit", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/audit?limit=200"
			}
			return "/v1/admin/audit?limit=1000"
		}},
		{op: "admin.events_history", path: func(w *Worker) string {
			if 0 == w.Pick(2) {
				return "/v1/admin/events/history?limit=200"
			}
			return "/v1/admin/events/history?limit=1000"
		}},
		{op: "admin.transfers", path: fixedPath("/v1/admin/transfers")},
		{op: "admin.jobs", path: func(w *Worker) string {
			switch w.Pick(3) {
			case 0:
				return "/v1/admin/jobs"
			case 1:
				return "/v1/admin/jobs?limit=200&state=succeeded"
			}
			return "/v1/admin/jobs?kind=usage_refresh"
		}},
		{op: "admin.workers", path: fixedPath("/v1/admin/workers")},
	}
}

func init() {
	table := adminReadTable()

	register(Scenario{
		Name:  "admin-read",
		Group: "admin",
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			for {
				entry := table[w.Pick(len(table))]
				if nil != entry.needs && !entry.needs(env.Features) {
					continue
				}
				_, err := env.Admin.Do(ctx, Request{Op: entry.op, Method: http.MethodGet, Path: entry.path(w), Expect: entry.expect})
				return err
			}
		},
	})

	register(Scenario{
		Name:  "admin-write",
		Group: "admin",
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			switch w.Pick(7) {
			case 0, 1:
				return adminUserLifecycle(ctx, env, w)
			case 2:
				return adminCacheInvalidate(ctx, env, w)
			case 3:
				_, err := env.Admin.Do(ctx, Request{
					Op:     "admin.config_validate",
					Method: http.MethodPost,
					Path:   "/v1/admin/config/validate",
					JSON:   map[string]any{"changes": map[string]any{"server.read_timeout": fmt.Sprintf("%ds", 30+w.Pick(30))}},
					Expect: []int{http.StatusOK},
				})
				return err
			case 4:
				if _, err := env.Admin.Do(ctx, Request{Op: "admin.ratelimits", Method: http.MethodGet, Path: "/v1/admin/ratelimits", Expect: cacheOrDisabled}); nil != err {
					return err
				}
				_, err := env.Admin.Do(ctx, Request{Op: "admin.hygiene", Method: http.MethodGet, Path: "/v1/admin/hygiene"})
				return err
			case 5:
				return adminWorkerRun(ctx, env)
			}
			_, err := env.Admin.Do(ctx, Request{Op: "admin.users", Method: http.MethodGet, Path: "/v1/admin/users"})
			return err
		},
	})

	register(Scenario{
		Name:  "account",
		Group: "admin",
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			var err error
			switch w.Pick(9) {
			case 0:
				_, err = w.Client.Do(ctx, Request{Op: "account.me", Method: http.MethodGet, Path: "/v1/me"})
			case 1:
				_, err = w.Client.Do(ctx, Request{Op: "account.capabilities", Method: http.MethodGet, Path: "/v1/capabilities"})
			case 2:
				_, err = w.Client.Do(ctx, Request{Op: "account.usage", Method: http.MethodGet, Path: "/v1/usage?path=%2F&depth=0"})
			case 3:
				_, err = w.Client.Do(ctx, Request{Op: "account.usage_deep", Method: http.MethodGet, Path: fmt.Sprintf("/v1/usage?path=%%2F&depth=%d&histograms=true", 1+w.Pick(3))})
			case 4:
				perms := []string{"read", "write", "delete"}
				paths := []string{"/", env.Path("account", "probe", w.Name("f"))}
				_, err = w.Client.Do(ctx, Request{
					Op:     "account.permission_check",
					Method: http.MethodGet,
					Path:   "/v1/permissions/check?perm=" + perms[w.Pick(len(perms))] + "&path=" + url.QueryEscape(paths[w.Pick(len(paths))]),
				})
			case 5:
				_, err = env.Admin.Do(ctx, Request{
					Op:     "account.permission_check_user",
					Method: http.MethodGet,
					Path:   "/v1/permissions/check?perm=read&user=" + url.QueryEscape("perf-probe") + "&path=" + url.QueryEscape(env.Path("account", "probe")),
				})
			case 6:
				_, err = w.Client.Do(ctx, Request{Op: "account.jobs", Method: http.MethodGet, Path: "/v1/jobs?limit=100"})
			case 7:
				_, err = w.Client.Do(ctx, Request{Op: "account.jobs_filtered", Method: http.MethodGet, Path: "/v1/jobs?limit=50&state=succeeded,failed"})
			default:
				_, err = w.Client.Do(ctx, Request{Op: "account.me_events", Method: http.MethodGet, Path: "/v1/me/events?limit=100"})
			}
			return err
		},
	})

	register(Scenario{
		Name:  "health",
		Group: "admin",
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			if 0 == w.Pick(2) {
				_, err := w.Client.Do(ctx, Request{Op: "health.live", Method: http.MethodGet, Path: "/health", Expect: []int{http.StatusOK}})
				return err
			}
			_, err := w.Client.Do(ctx, Request{Op: "health.ready", Method: http.MethodGet, Path: "/health/ready", Expect: []int{http.StatusOK, http.StatusServiceUnavailable}})
			return err
		},
	})
}

func adminUserLifecycle(ctx context.Context, env *Env, w *Worker) error {
	id := w.Name("perfuser")
	userPath := "/v1/admin/users/" + id
	var created struct {
		UserID string `json:"user_id"`
		APIKey string `json:"api_key"`
	}
	if err := env.Admin.JSON(ctx, Request{
		Op:     "admin.user_create",
		Method: http.MethodPost,
		Path:   "/v1/admin/users",
		JSON:   map[string]any{"user_id": id, "is_admin": false},
		Expect: []int{http.StatusCreated},
	}, &created); nil != err {
		return fmt.Errorf("create user: %w", err)
	}
	cleanup := func() {
		_, _ = env.Admin.Do(context.WithoutCancel(ctx), Request{Op: "admin.user_delete", Method: http.MethodDelete, Path: userPath, Expect: []int{http.StatusNoContent, http.StatusNotFound, http.StatusConflict}})
	}
	if _, err := env.Admin.Do(ctx, Request{
		Op:     "admin.user_rotate",
		Method: http.MethodPost,
		Path:   userPath + "/rotate",
		Expect: []int{http.StatusOK},
	}); nil != err {
		cleanup()
		return fmt.Errorf("rotate user: %w", err)
	}
	for _, admin := range []bool{true, false} {
		if _, err := env.Admin.Do(ctx, Request{
			Op:     "admin.user_patch",
			Method: http.MethodPatch,
			Path:   userPath,
			JSON:   map[string]any{"is_admin": admin},
			Expect: []int{http.StatusOK},
		}); nil != err {
			cleanup()
			return fmt.Errorf("patch user: %w", err)
		}
	}
	if _, err := env.Admin.Do(ctx, Request{
		Op:     "admin.user_delete",
		Method: http.MethodDelete,
		Path:   userPath,
		Expect: []int{http.StatusNoContent},
	}); nil != err {
		cleanup()
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func adminCacheInvalidate(ctx context.Context, env *Env, w *Worker) error {
	body := map[string]any{"prefix": env.Path("admin-write")}
	if 0 == w.Pick(8) {
		body = map[string]any{"all": true}
	}
	_, err := env.Admin.Do(ctx, Request{
		Op:     "admin.cache_invalidate",
		Method: http.MethodPost,
		Path:   "/v1/admin/cache/invalidate",
		JSON:   body,
		Expect: []int{http.StatusOK},
	})
	return err
}

func adminWorkerRun(ctx context.Context, env *Env) error {
	var accepted struct {
		JobID string `json:"job_id"`
	}
	resp, err := env.Admin.Do(ctx, Request{
		Op:     "admin.worker_run",
		Method: http.MethodPost,
		Path:   "/v1/admin/workers/usage_refresh/run",
		Expect: []int{http.StatusAccepted, http.StatusConflict},
	})
	if nil != err {
		return fmt.Errorf("run worker: %w", err)
	}
	if http.StatusAccepted != resp.Status {
		return nil
	}
	if err := json.Unmarshal(resp.Body, &accepted); nil != err {
		return fmt.Errorf("decode worker run: %w", err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var job struct {
			State string `json:"state"`
		}
		if err := env.Admin.JSON(ctx, Request{
			Op:     "admin.job_poll",
			Method: http.MethodGet,
			Path:   "/v1/admin/jobs/" + url.PathEscape(accepted.JobID),
		}, &job); nil != err {
			return fmt.Errorf("poll job: %w", err)
		}
		switch job.State {
		case "succeeded", "partial", "failed", "cancelled":
			if "failed" == job.State {
				return fmt.Errorf("worker job %s failed", accepted.JobID)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("worker job %s did not finish", accepted.JobID)
}
