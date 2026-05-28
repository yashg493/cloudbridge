package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

// JobHandler exposes sync job read and cancellation endpoints.
type JobHandler struct {
	syncJobRepo *store.SyncJobRepo
	logger      *zap.Logger
}

// NewJobHandler creates a JobHandler.
func NewJobHandler(syncJobRepo *store.SyncJobRepo, logger *zap.Logger) *JobHandler {
	return &JobHandler{syncJobRepo: syncJobRepo, logger: logger}
}

// List handles GET /api/v1/jobs.
// Supports query params: status, page, per_page.
func (h *JobHandler) List(c *gin.Context) {
	status := c.Query("status")
	page, perPage, offset := parsePage(c)
	ctx := c.Request.Context()

	total, err := h.syncJobRepo.Count(ctx, status)
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	jobs, err := h.syncJobRepo.List(ctx, status, perPage, offset)
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	okList(c, jobs, int(total), page, perPage)
}

// Get handles GET /api/v1/jobs/:id.
func (h *JobHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "job id must be a valid UUID")
		return
	}
	job, err := h.syncJobRepo.GetByID(c.Request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "job not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	ok(c, job)
}

// Cancel handles DELETE /api/v1/jobs/:id.
// Transitions the job to "cancelled". Returns 409 if the job is already terminal.
func (h *JobHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "job id must be a valid UUID")
		return
	}
	ctx := c.Request.Context()

	job, err := h.syncJobRepo.GetByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "job not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}

	terminal := job.Status == models.SyncStatusCompleted ||
		job.Status == models.SyncStatusFailed ||
		job.Status == models.SyncStatusCancelled

	if terminal {
		conflict(c, "job is already in a terminal state and cannot be cancelled")
		return
	}

	if err := h.syncJobRepo.UpdateStatus(ctx, id, string(models.SyncStatusCancelled), ""); err != nil {
		internalError(c, h.logger, err)
		return
	}
	ok(c, gin.H{"message": "job cancelled", "job_id": id})
}
