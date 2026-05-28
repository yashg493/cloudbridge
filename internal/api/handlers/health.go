package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/worker"
)

const version = "1.0.0"

// HealthHandler serves the /health endpoint and legacy k8s probes.
type HealthHandler struct {
	pool       *pgxpool.Pool
	workerPool *worker.WorkerPool
	logger     *zap.Logger
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(pool *pgxpool.Pool, workerPool *worker.WorkerPool, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{pool: pool, workerPool: workerPool, logger: logger}
}

// Health handles GET /health.
// Returns system status including DB connectivity and active worker count.
func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()

	dbStatus := "connected"
	overall := http.StatusOK

	if h.pool == nil {
		dbStatus = "not initialised"
		overall = http.StatusServiceUnavailable
	} else if err := h.pool.Ping(ctx); err != nil {
		h.logger.Warn("health: db ping failed", zap.Error(err))
		dbStatus = "error: " + err.Error()
		overall = http.StatusServiceUnavailable
	}

	var activeWorkers int32
	if h.workerPool != nil {
		activeWorkers = h.workerPool.ActiveWorkers()
	}

	status := "ok"
	if overall != http.StatusOK {
		status = "degraded"
	}

	c.JSON(overall, Response{
		Data: gin.H{
			"status":         status,
			"db":             dbStatus,
			"active_workers": activeWorkers,
			"version":        version,
		},
	})
}
