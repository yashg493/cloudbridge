package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

// FileHandler handles file CRUD and tiering endpoints.
type FileHandler struct {
	fileRepo   *store.FileRepo
	workerPool *worker.WorkerPool
	logger     *zap.Logger
}

// NewFileHandler creates a FileHandler.
func NewFileHandler(
	fileRepo *store.FileRepo,
	workerPool *worker.WorkerPool,
	logger *zap.Logger,
) *FileHandler {
	return &FileHandler{fileRepo: fileRepo, workerPool: workerPool, logger: logger}
}

// Upload handles POST /api/v1/namespaces/:namespace_id/files
// Registers file metadata and optionally enqueues a sync job.
func (h *FileHandler) Upload(c *gin.Context) {
	// TODO: parse and validate namespace_id path param (uuid.Parse)
	// TODO: bind request body — support both JSON metadata and multipart form
	// TODO: validate required fields: name, path, size_bytes
	// TODO: generate file UUID, set tier=hot, status=active, accessed_at=now
	// TODO: compute or accept SHA-256 checksum
	// TODO: h.fileRepo.Create(c.Request.Context(), &file)
	// TODO: if cloud sync requested, h.workerPool.Submit(worker.NewSyncJob(...))
	// TODO: c.JSON(http.StatusCreated, file)
	h.logger.Warn("Upload handler not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Get handles GET /api/v1/namespaces/:namespace_id/files/:file_id
func (h *FileHandler) Get(c *gin.Context) {
	// TODO: parse namespace_id and file_id path params
	// TODO: h.fileRepo.GetByID(c.Request.Context(), fileID)
	// TODO: verify file.NamespaceID matches namespace_id (prevents cross-namespace access)
	// TODO: fire-and-forget TouchAccessedAt in background goroutine
	// TODO: on pgx.ErrNoRows → 404; other errors → 500
	// TODO: c.JSON(http.StatusOK, file)
	h.logger.Warn("Get handler not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// List handles GET /api/v1/namespaces/:namespace_id/files
// Supports query params: limit (default 50, max 200), offset (default 0).
func (h *FileHandler) List(c *gin.Context) {
	// TODO: parse namespace_id path param
	// TODO: parse and clamp limit / offset query params
	// TODO: h.fileRepo.ListByNamespace(c.Request.Context(), nsID, limit, offset)
	// TODO: return {"data": [...], "limit": n, "offset": m, "count": len}
	h.logger.Warn("List handler not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Delete handles DELETE /api/v1/namespaces/:namespace_id/files/:file_id
// Soft-deletes the metadata record and enqueues cloud cleanup if tiered.
func (h *FileHandler) Delete(c *gin.Context) {
	// TODO: parse namespace_id and file_id
	// TODO: h.fileRepo.GetByID to confirm existence and namespace ownership
	// TODO: h.fileRepo.Delete(c.Request.Context(), fileID)
	// TODO: if file.Tier != hot, enqueue SyncJobTypeDelete job
	// TODO: c.Status(http.StatusNoContent)
	h.logger.Warn("Delete handler not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Tier handles POST /api/v1/namespaces/:namespace_id/files/:file_id/tier
// Body: {"target_tier": "warm"|"cold"|"hot"}
// Returns 202 Accepted with a job_id; actual transition happens asynchronously.
func (h *FileHandler) Tier(c *gin.Context) {
	// TODO: parse namespace_id and file_id
	// TODO: bind request body: target_tier
	// TODO: validate tier transition is legal (hot→warm, hot→cold, warm→cold, *→hot)
	// TODO: h.fileRepo.GetByID to confirm file exists and is not already on target tier
	// TODO: build worker.NewSyncJob(fileID, targetTier, string(jobType), ...)
	// TODO: h.workerPool.Submit(job) — return 429 if ErrQueueFull
	// TODO: c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID(), "target_tier": targetTier})
	h.logger.Warn("Tier handler not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
