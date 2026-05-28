package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "cloudbridge"

// syncDurationBuckets matches the spec: .1, .5, 1, 5, 10, 30, 60 seconds.
var syncDurationBuckets = []float64{.1, .5, 1, 5, 10, 30, 60}

// Registry holds every Prometheus metric published by CloudBridge.
// Instantiate once in main and inject where needed.
type Registry struct {
	// ── Counters ─────────────────────────────────────────────────────────────
	FilesRegisteredTotal  *prometheus.CounterVec // labels: namespace, protocol
	SyncJobsTotal         *prometheus.CounterVec // labels: operation, status
	BytesTransferredTotal *prometheus.CounterVec // labels: operation, backend

	// ── Histograms ───────────────────────────────────────────────────────────
	SyncDurationSeconds *prometheus.HistogramVec // labels: operation; buckets: .1,.5,1,5,10,30,60
	APIRequestDuration  *prometheus.HistogramVec // labels: method, path, status

	// ── Gauges ───────────────────────────────────────────────────────────────
	ActiveWorkers   prometheus.Gauge
	PendingJobs     prometheus.Gauge
	NamespacesTotal *prometheus.GaugeVec // labels: status
	FilesByTier     *prometheus.GaugeVec // labels: tier
}

// NewRegistry registers and returns all CloudBridge Prometheus metrics.
// Uses promauto so all metrics are automatically registered with the default registerer.
func NewRegistry() *Registry {
	return &Registry{
		// Counters
		FilesRegisteredTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "files_registered_total",
			Help:      "Total files registered, partitioned by namespace and protocol.",
		}, []string{"namespace", "protocol"}),

		SyncJobsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sync_jobs_total",
			Help:      "Total sync jobs partitioned by operation and terminal status.",
		}, []string{"operation", "status"}),

		BytesTransferredTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_transferred_total",
			Help:      "Total bytes transferred to/from cloud storage, partitioned by operation and backend.",
		}, []string{"operation", "backend"}),

		// Histograms
		SyncDurationSeconds: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "sync_duration_seconds",
			Help:      "Wall-clock time for a sync operation to complete.",
			Buckets:   syncDurationBuckets,
		}, []string{"operation"}),

		APIRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "api_request_duration_seconds",
			Help:      "HTTP API request latency partitioned by method, path template, and status code.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),

		// Gauges
		ActiveWorkers: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_workers",
			Help:      "Number of worker goroutines currently executing a sync job.",
		}),

		PendingJobs: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_jobs",
			Help:      "Current number of sync jobs buffered in the worker pool channel.",
		}),

		NamespacesTotal: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "namespaces_total",
			Help:      "Current number of namespaces by status.",
		}, []string{"status"}),

		FilesByTier: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "files_by_tier",
			Help:      "Current number of files in each storage tier.",
		}, []string{"tier"}),
	}
}
