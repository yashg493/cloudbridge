package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/store"
)

// HealthHandler serves Kubernetes liveness and readiness probes.
type HealthHandler struct {
	db     *store.DB
	logger *zap.Logger
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(db *store.DB, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{db: db, logger: logger}
}

// Liveness handles GET /healthz.
// Returns 200 as long as the process is running; does not check dependencies.
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness handles GET /readyz.
// Returns 200 only when all critical dependencies are healthy.
// Returns 503 with a JSON body describing which checks failed.
func (h *HealthHandler) Readiness(c *gin.Context) {
	checks := map[string]string{}
	healthy := true

	// Database check
	if h.db != nil {
		if err := h.db.HealthCheck(c.Request.Context()); err != nil {
			h.logger.Warn("readiness: db check failed", zap.Error(err))
			checks["database"] = err.Error()
			healthy = false
		} else {
			checks["database"] = "ok"
		}
	} else {
		// TODO: remove this branch once db is always initialised in main
		checks["database"] = "not initialised"
		healthy = false
	}

	// TODO: add cloud provider connectivity check
	// TODO: add worker pool liveness check

	if !healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "checks": checks})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "checks": checks})
}
