package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/api/handlers"
	"github.com/yashg493/cloudbridge/internal/api/middleware"
	"github.com/yashg493/cloudbridge/internal/metrics"
	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

// RouterConfig bundles all dependencies required to wire the HTTP routes.
type RouterConfig struct {
	Logger     *zap.Logger
	DB         *store.DB
	FileRepo   *store.FileRepository
	NSRepo     *store.NamespaceRepository
	WorkerPool *worker.Pool
	MetricsReg *metrics.Registry
}

// NewRouter creates a fully configured Gin engine.
// gin.New() is used instead of gin.Default() so we control all middleware.
func NewRouter(cfg RouterConfig) *gin.Engine {
	r := gin.New()

	// ── Global middleware (order matters) ────────────────────────────────────
	r.Use(middleware.Recovery(cfg.Logger))
	r.Use(middleware.RequestLogger(cfg.Logger))
	// TODO: add Prometheus HTTP metrics middleware using cfg.MetricsReg
	//       record cloudbridge_http_requests_total and cloudbridge_http_request_duration_seconds

	// ── Observability endpoints ──────────────────────────────────────────────
	healthHandler := handlers.NewHealthHandler(cfg.DB, cfg.Logger)
	r.GET("/healthz", healthHandler.Liveness)
	r.GET("/readyz", healthHandler.Readiness)
	r.GET("/metrics", gin.WrapH(promhttp.Handler())) // Prometheus scrape endpoint

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")
	{
		// Namespace routes
		nsHandler := handlers.NewNamespaceHandler(cfg.NSRepo, cfg.Logger)
		ns := v1.Group("/namespaces")
		{
			ns.POST("", nsHandler.Create)
			ns.GET("", nsHandler.List)
			ns.GET("/:namespace_id", nsHandler.Get)
			ns.PUT("/:namespace_id", nsHandler.Update)
			ns.DELETE("/:namespace_id", nsHandler.Delete)

			// File routes nested under a namespace
			fileHandler := handlers.NewFileHandler(cfg.FileRepo, cfg.WorkerPool, cfg.Logger)
			files := ns.Group("/:namespace_id/files")
			{
				files.POST("", fileHandler.Upload)
				files.GET("", fileHandler.List)
				files.GET("/:file_id", fileHandler.Get)
				files.DELETE("/:file_id", fileHandler.Delete)
				files.POST("/:file_id/tier", fileHandler.Tier) // async tier trigger
			}
		}
	}

	return r
}
