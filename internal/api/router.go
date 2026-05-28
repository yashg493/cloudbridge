package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/api/handlers"
	"github.com/yashg493/cloudbridge/internal/api/middleware"
	"github.com/yashg493/cloudbridge/internal/metrics"
	"github.com/yashg493/cloudbridge/internal/nfs"
	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

// RouterConfig bundles all dependencies required to wire the HTTP routes.
type RouterConfig struct {
	Logger      *zap.Logger
	Pool        *pgxpool.Pool
	FileRepo    *store.FileRepo
	NSRepo      *store.NamespaceRepo
	SyncJobRepo *store.SyncJobRepo
	WorkerPool  *worker.WorkerPool
	NFSSim      *nfs.Simulator
	MetricsReg  *metrics.Registry
}

// NewRouter creates a fully configured Gin engine.
// gin.New() is used instead of gin.Default() so we control all middleware explicitly.
func NewRouter(cfg RouterConfig) *gin.Engine {
	r := gin.New()

	// ── Global middleware ───────────────────────────────────────────────────────────────
	r.Use(middleware.Recovery(cfg.Logger))
	r.Use(middleware.RequestLogger(cfg.Logger))
	r.Use(middleware.CORS())

	// ── Observability endpoints ──────────────────────────────────────────────────
	healthHandler := handlers.NewHealthHandler(cfg.Pool, cfg.WorkerPool, cfg.Logger)
	r.GET("/health", healthHandler.Health)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// ─ Namespace routes ───────────────────────────────────────────────────────
	nsHandler := handlers.NewNamespaceHandler(
		cfg.NSRepo, cfg.FileRepo, cfg.SyncJobRepo,
		cfg.NFSSim, cfg.WorkerPool, cfg.Logger,
	)
	v1.POST("/namespaces", nsHandler.Create)
	v1.GET("/namespaces", nsHandler.List)
	v1.GET("/namespaces/:id", nsHandler.Get)
	v1.DELETE("/namespaces/:id", nsHandler.Delete)
	v1.GET("/namespaces/:id/stats", nsHandler.Stats)
	v1.POST("/namespaces/:id/sync", nsHandler.Sync)

	// ─ File routes (nested under namespace) ─────────────────────────────────
	// Note: Gin requires wildcard (*path) routes to be registered last within a group
	// to avoid conflicts with other /:id sub-routes.
	fileHandler := handlers.NewFileHandler(
		cfg.FileRepo, cfg.NSRepo, cfg.SyncJobRepo,
		cfg.NFSSim, cfg.WorkerPool, cfg.Logger,
	)
	v1.POST("/namespaces/:id/files", fileHandler.Register)
	v1.GET("/namespaces/:id/files", fileHandler.List)
	v1.GET("/namespaces/:id/files/*path", fileHandler.GetByPath)
	v1.DELETE("/namespaces/:id/files/*path", fileHandler.Delete)

	// ─ Sync job routes ──────────────────────────────────────────────────────
	jobHandler := handlers.NewJobHandler(cfg.SyncJobRepo, cfg.Logger)
	v1.GET("/jobs", jobHandler.List)
	v1.GET("/jobs/:id", jobHandler.Get)
	v1.DELETE("/jobs/:id", jobHandler.Cancel)

	return r
}
