// Package metrics provides Prometheus metrics for CallFS operations.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP request metrics
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

	// Backend operation metrics
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

	// Metadata database metrics
	MetadataDBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_metadata_db_queries_total",
			Help: "Total number of metadata database queries",
		},
		[]string{"operation"},
	)

	MetadataDBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_metadata_db_query_duration_seconds",
			Help:    "Metadata database query duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// Single-use link metrics
	SingleUseLinkGenerationsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "callfs_single_use_link_generations_total",
			Help: "Total number of single-use links generated",
		},
	)

	SingleUseLinkConsumptionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_single_use_link_consumptions_total",
			Help: "Total number of single-use links consumed",
		},
		[]string{"status"}, // "success", "expired", "invalid", "not_found"
	)

	// Lock manager metrics
	LockOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_lock_operations_total",
			Help: "Total number of lock operations",
		},
		[]string{"operation", "status"}, // operation: "acquire", "release"; status: "success", "failure"
	)

	LockOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "callfs_lock_operation_duration_seconds",
			Help:    "Lock operation duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// Active locks gauge
	ActiveLocks = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "callfs_active_locks",
			Help: "Number of currently active locks",
		},
	)

	// File operations metrics
	FileOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_file_operations_total",
			Help: "Total number of file operations",
		},
		[]string{"operation", "backend_type"}, // operation: "create", "read", "update", "delete"
	)

	// Error metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "callfs_errors_total",
			Help: "Total number of errors by component",
		},
		[]string{"component", "error_type"},
	)
)

// RegisterMetrics ensures all metrics are registered with Prometheus.
// This function is idempotent and safe to call multiple times.
func RegisterMetrics() {
	// All metrics are automatically registered via promauto.
	// This function exists for explicit initialization if needed.
}
