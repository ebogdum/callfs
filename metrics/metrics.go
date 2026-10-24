package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var knownMethods = map[string]struct{}{
	"GET":     {},
	"HEAD":    {},
	"POST":    {},
	"PUT":     {},
	"PATCH":   {},
	"DELETE":  {},
	"OPTIONS": {},
}

func NormalizeMethod(method string) string {
	if _, ok := knownMethods[method]; ok {
		return method
	}
	return "OTHER"
}

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	BackendOpsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_backend_ops_total",
			Help: "Total number of backend operations",
		},
		[]string{"backend_type", "operation"},
	)

	BackendOpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_backend_op_duration_seconds",
			Help:    "Backend operation duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"backend_type", "operation"},
	)

	BackendOpFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_backend_op_failures_total",
			Help: "Total number of failed backend operations",
		},
		[]string{"backend_type", "operation"},
	)

	MetadataDBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_metadata_db_queries_total",
			Help: "Total number of metadata store queries",
		},
		[]string{"operation"},
	)

	MetadataDBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_metadata_db_query_duration_seconds",
			Help:    "Metadata store query duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	SingleUseLinkGenerationsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "callfs_single_use_link_generations_total",
			Help: "Total number of single-use links generated",
		},
	)

	SingleUseLinksActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "callfs_single_use_links_active",
			Help: "Number of download links that can still be opened",
		},
	)

	SingleUseLinkConsumptionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_single_use_link_consumptions_total",
			Help: "Total number of single-use links consumed",
		},
		[]string{"status"},
	)

	LockOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_lock_operations_total",
			Help: "Total number of edit lock operations",
		},
		[]string{"operation", "status"},
	)

	LockOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_lock_operation_duration_seconds",
			Help:    "Edit lock operation duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	ActiveLocks = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "callfs_active_locks",
			Help: "Number of edit locks currently held",
		},
	)

	FileOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_file_operations_total",
			Help: "Total number of file operations",
		},
		[]string{"operation", "backend_type"},
	)

	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_errors_total",
			Help: "Total number of errors returned to clients",
		},
		[]string{"component", "error_type"},
	)

	CacheLookupsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_cache_lookups_total",
			Help: "Total number of metadata cache lookups",
		},
		[]string{"result"},
	)

	CacheEvictionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "callfs_cache_evictions_total",
			Help: "Total number of metadata cache evictions",
		},
	)

	CacheEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "callfs_cache_entries",
			Help: "Number of entries in the metadata cache",
		},
	)

	RateLimitRejectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_rate_limit_rejections_total",
			Help: "Total number of requests rejected by rate limits",
		},
		[]string{"endpoint"},
	)

	ReplicationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_replication_total",
			Help: "Total number of backup copy attempts",
		},
		[]string{"result"},
	)

	TransfersActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "callfs_transfers_active",
			Help: "Number of file transfers currently running",
		},
		[]string{"kind"},
	)

	AuthFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_auth_failures_total",
			Help: "Total number of failed authentication attempts",
		},
		[]string{"reason"},
	)

	JobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_jobs_total",
			Help: "Total number of finished jobs",
		},
		[]string{"kind", "result"},
	)

	ErasurePiecesRepairedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "callfs_erasure_pieces_repaired_total",
			Help: "Total number of erasure pieces repaired",
		},
	)

	UsageBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "callfs_usage_bytes",
			Help: "Bytes stored per storage backend",
		},
		[]string{"storage"},
	)

	UsageFiles = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "callfs_usage_files",
			Help: "Files stored per storage backend",
		},
		[]string{"storage"},
	)

	OverwriteRestoresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "callfs_overwrite_restores_total",
			Help: "Local disk overwrites rolled back to the previous file copy",
		},
	)
)
