package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "cloudbridge"

// Registry holds every Prometheus metric published by CloudBridge.
// Instantiate once in main and inject where needed.
type Registry struct {
	// ── HTTP layer ──────────────────────────────────────────────────────────
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// ── File operations ─────────────────────────────────────────────────────
	FileUploadsTotal   prometheus.Counter
	FileDownloadsTotal prometheus.Counter
	FileDeletesTotal   prometheus.Counter

	// ── Tiering ─────────────────────────────────────────────────────────────
	TieringOperationsTotal *prometheus.CounterVec   // labels: direction, status
	TieringDurationSeconds *prometheus.HistogramVec // labels: direction
	FilesPerTier           *prometheus.GaugeVec     // labels: tier

	// ── Storage totals ───────────────────────────────────────────────────────
	StorageBytesTotal *prometheus.GaugeVec // labels: tier

	// ── Worker pool ──────────────────────────────────────────────────────────
	WorkerQueueDepth  prometheus.Gauge
	WorkerJobsTotal   *prometheus.CounterVec   // labels: type, status
	WorkerJobDuration *prometheus.HistogramVec // labels: type

	// ── NFS simulation ───────────────────────────────────────────────────────
	NFSOperationsTotal *prometheus.CounterVec   // labels: operation, status
	NFSLatencySeconds  *prometheus.HistogramVec // labels: operation

	// ── Database ─────────────────────────────────────────────────────────────
	DBQueryDuration *prometheus.HistogramVec // labels: query_name
	DBErrors        *prometheus.CounterVec   // labels: query_name
}

// NewRegistry registers and returns all CloudBridge Prometheus metrics.
// Uses promauto so metrics are automatically registered with the default registerer.
func NewRegistry() *Registry {
	return &Registry{
		// HTTP
		HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http",
			Name: "requests_total",
			Help: "Total HTTP requests partitioned by method, path template, and status code.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http",
			Name:    "request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		HTTPResponseSize: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http",
			Name:    "response_size_bytes",
			Help:    "HTTP response body size in bytes.",
			Buckets: prometheus.ExponentialBuckets(512, 4, 10),
		}, []string{"method", "path"}),

		// File ops
		FileUploadsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "file_uploads_total",
			Help: "Total number of file upload requests received.",
		}),
		FileDownloadsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "file_downloads_total",
			Help: "Total number of file download requests received.",
		}),
		FileDeletesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "file_deletes_total",
			Help: "Total number of file delete requests received.",
		}),

		// Tiering
		TieringOperationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "tiering",
			Name: "operations_total",
			Help: "Total tiering operations partitioned by direction (up/down) and status.",
		}, []string{"direction", "status"}),

		TieringDurationSeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "tiering",
			Name:    "duration_seconds",
			Help:    "Wall-clock time for a tiering operation to complete.",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		}, []string{"direction"}),

		FilesPerTier: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "files_per_tier",
			Help:      "Current number of active files residing in each storage tier.",
		}, []string{"tier"}),

		// Storage
		StorageBytesTotal: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "storage_bytes_total",
			Help:      "Total bytes of active file content per tier.",
		}, []string{"tier"}),

		// Worker pool
		WorkerQueueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "worker",
			Name: "queue_depth",
			Help: "Current number of jobs buffered in the worker pool channel.",
		}),

		WorkerJobsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "worker",
			Name: "jobs_total",
			Help: "Total worker jobs partitioned by job type and final status.",
		}, []string{"type", "status"}),

		WorkerJobDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "worker",
			Name:    "job_duration_seconds",
			Help:    "Time taken to execute a worker job.",
			Buckets: prometheus.DefBuckets,
		}, []string{"type"}),

		// NFS
		NFSOperationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "nfs",
			Name: "operations_total",
			Help: "Total simulated NFS operations partitioned by procedure and status.",
		}, []string{"operation", "status"}),

		NFSLatencySeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "nfs",
			Name:    "latency_seconds",
			Help:    "Simulated NFS operation latency.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		}, []string{"operation"}),

		// Database
		DBQueryDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "db",
			Name:    "query_duration_seconds",
			Help:    "PostgreSQL query execution time.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		}, []string{"query_name"}),

		DBErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "db",
			Name: "errors_total",
			Help: "Total PostgreSQL errors partitioned by query name.",
		}, []string{"query_name"}),
	}
}
