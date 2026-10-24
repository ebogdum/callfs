package admin

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/findings"
	"github.com/ebogdum/callfs/v2/health"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/links"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/handlers"
	"github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
)

type Deps struct {
	Engine        *core.Engine
	Store         metadata.Store
	Erasure       *erasure.Manager
	Config        *config.AppConfig
	ConfigPath    string
	ConfigSources config.SourceMap
	Authenticator *auth.APIKeyAuthenticator
	Admins        *auth.AdminSet
	Links         *links.LinkManager
	RateLimits    *middleware.RateLimitRegistry
	Locks         locks.Inspector
	Telemetry     *telemetry.Hub
	Sampler       *telemetry.Sampler
	Transfers     TransferSource
	Jobs          *jobs.Runner
	Trash         *core.Trash
	Uploads       *core.Uploads
	Health        *health.Checker
	Findings      *findings.Engine
	Restart       func(reason string) error
	Logger        *zap.Logger
	ClusterLog    ClusterMetadataLog
	NATS          *nats.Conn
	JetStream     jetstream.JetStream
	ClusterName   string
	Presence      cluster.Presence
	DeadTasks     cluster.DeadTasks
}

type API struct {
	Deps
	deps        Deps
	factsMemory factsMemory
	removePeer  func(ctx context.Context, peer string) error
}

type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

func NewAPI(deps Deps) *API {
	return &API{Deps: deps, deps: deps}
}

func Mount(r chi.Router, api *API) {
	r.Route("/admin", func(admin chi.Router) {
		admin.Use(middleware.RequireAdmin(api.deps.Admins))
		for _, entry := range api.routes() {
			admin.MethodFunc(entry.method, entry.pattern, entry.handler)
		}
	})
}

func (a *API) routes() []route {
	return []route{
		{http.MethodGet, "/status", a.Status},
		{http.MethodPost, "/restart", a.Restart},
		{http.MethodGet, "/migrations", a.Migrations},
		{http.MethodGet, "/health", a.Health},
		{http.MethodGet, "/findings", a.Findings},
		{http.MethodPost, "/findings/{id}/mute", a.MuteFinding},
		{http.MethodDelete, "/findings/{id}/mute", a.UnmuteFinding},
		{http.MethodPost, "/findings/{id}/ignore", a.IgnoreFinding},
		{http.MethodDelete, "/findings/{id}/ignore", a.UnignoreFinding},
		{http.MethodGet, "/tls", a.TLS},
		{http.MethodGet, "/config", a.GetConfig},
		{http.MethodGet, "/config/schema", a.ConfigSchema},
		{http.MethodPost, "/config/validate", a.ValidateConfig},
		{http.MethodPut, "/config", a.SaveConfig},
		{http.MethodPatch, "/config/runtime", a.PatchRuntimeConfig},
		{http.MethodGet, "/config/export", a.ExportConfig},
		{http.MethodGet, "/cluster", a.Cluster},
		{http.MethodGet, "/cluster/state-stats", a.ClusterStateStats},
		{http.MethodPost, "/cluster/leadership-transfer", a.ClusterLeadershipTransfer},
		{http.MethodDelete, "/cluster/nodes/{id}", a.ClusterRemoveNode},
		{http.MethodGet, "/cluster/snapshots", a.ClusterSnapshots},
		{http.MethodPost, "/cluster/snapshot", a.ClusterTakeSnapshot},
		{http.MethodGet, "/cluster/join-config", a.ClusterJoinConfig},
		{http.MethodGet, "/instances", a.Instances},
		{http.MethodPut, "/instances/{id}/endpoint", a.SetInstanceEndpoint},
		{http.MethodDelete, "/instances/{id}/endpoint", a.RemoveInstanceEndpoint},
		{http.MethodPost, "/instances/{id}/probe", a.ProbeInstance},
		{http.MethodGet, "/backends", a.Backends},
		{http.MethodPost, "/backends/{name}/probe", a.ProbeBackend},
		{http.MethodGet, "/erasure", a.ErasureSummary},
		{http.MethodPost, "/erasure/plan", a.ErasurePlan},
		{http.MethodPost, "/erasure/verify-all", a.ErasureVerifyAll},
		{http.MethodGet, "/replication", a.Replication},
		{http.MethodPost, "/replication/retry", a.RetryReplication},
		{http.MethodGet, "/staging", a.Staging},
		{http.MethodDelete, "/staging", a.CleanStaging},
		{http.MethodGet, "/hygiene", a.Hygiene},
		{http.MethodPost, "/retag", a.Retag},
		{http.MethodGet, "/locks", a.Locks},
		{http.MethodDelete, "/locks/{key}", a.ForceReleaseLock},
		{http.MethodGet, "/cache", a.Cache},
		{http.MethodPost, "/cache/invalidate", a.InvalidateCache},
		{http.MethodGet, "/ratelimits", a.RateLimits},
		{http.MethodGet, "/users", a.Users},
		{http.MethodPost, "/users", a.CreateUser},
		{http.MethodPost, "/users/migrate-positional", a.MigratePositional},
		{http.MethodPatch, "/users/{id}", a.UpdateUser},
		{http.MethodDelete, "/users/{id}", a.DeleteUser},
		{http.MethodPost, "/users/{id}/rotate", a.RotateUserKey},
		{http.MethodGet, "/owners", a.Owners},
		{http.MethodPost, "/owners/reassign", a.ReassignOwner},
		{http.MethodGet, "/auth/failures", a.AuthFailures},
		{http.MethodGet, "/metrics", a.Metrics},
		{http.MethodGet, "/metrics/series", a.MetricSeries},
		{http.MethodGet, "/logs", a.Logs},
		{http.MethodGet, "/logs/stream", a.LogStream},
		{http.MethodGet, "/audit", a.Audit},
		{http.MethodGet, "/events", a.EventStream},
		{http.MethodGet, "/events/history", a.EventHistory},
		{http.MethodGet, "/transfers", a.Transfers},
		{http.MethodGet, "/jobs", a.Jobs},
		{http.MethodGet, "/jobs/{id}", a.Job},
		{http.MethodPost, "/jobs/{id}/cancel", a.CancelJob},
		{http.MethodPost, "/jobs/{id}/retry-failed", a.RetryJobFailed},
		{http.MethodGet, "/workers", a.Workers},
		{http.MethodPost, "/workers/{name}/run", a.RunWorker},
		{http.MethodGet, "/tasks/dead", a.DeadTasks},
		{http.MethodPost, "/tasks/dead/{id}/retry", a.RetryDeadTask},
		{http.MethodDelete, "/tasks/dead/{id}", a.DropDeadTask},
	}
}

func (a *API) logger() *zap.Logger {
	if nil == a.deps.Logger {
		return zap.NewNop()
	}
	return a.deps.Logger
}

func (a *API) unavailable(w http.ResponseWriter, component string) {
	handlers.SendErrorResponse(w, a.logger(), fmt.Errorf("%s is not running", component), http.StatusServiceUnavailable)
}
