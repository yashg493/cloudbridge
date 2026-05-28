package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/nfs"
	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

// FileHandler handles file registration, listing, retrieval, and deletion.
type FileHandler struct {
	fileRepo    *store.FileRepo
	nsRepo      *store.NamespaceRepo
	syncJobRepo *store.SyncJobRepo
	nfsSim      *nfs.Simulator
	workerPool  *worker.WorkerPool
	logger      *zap.Logger
}

// NewFileHandler creates a FileHandler.
func NewFileHandler(
	fileRepo *store.FileRepo,
	nsRepo *store.NamespaceRepo,
	syncJobRepo *store.SyncJobRepo,
	nfsSim *nfs.Simulator,
	workerPool *worker.WorkerPool,
	logger *zap.Logger,
) *FileHandler {
	return &FileHandler{
		fileRepo:    fileRepo,
		nsRepo:      nsRepo,
		syncJobRepo: syncJobRepo,
		nfsSim:      nfsSim,
		workerPool:  workerPool,
		logger:      logger,
	}
}

type registerFileReq struct {
	Path string `json:"path" binding:"required"`
}

// Register handles POST /api/v1/namespaces/:id/files.
// Reads file metadata from the NFS simulator, upserts it in the DB,
// and enqueues an upload job if the namespace has a cloud backend.
func (h *FileHandler) Register(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	var req registerFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid_body", err.Error())
		return
	}
	ctx := c.Request.Context()

	ns, err := h.nsRepo.GetByID(ctx, nsID)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "namespace not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}

	// Stat the file in NFS (path is relative to namespace root).
	nfsPath := ns.Name + "/" + strings.TrimPrefix(req.Path, "/")
	var sizeBytes int64
	var checksum string
	if h.nfsSim != nil {
		info, serr := h.nfsSim.Stat(nfsPath)
		if serr != nil {
			h.logger.Warn("register: NFS stat failed — using zero values",
				zap.String("path", nfsPath), zap.Error(serr))
		} else {
			sizeBytes = info.SizeBytes
			checksum = info.Checksum
		}
	}

	fileID := uuid.New()
	if existing, err := h.fileRepo.GetByPath(ctx, nsID, req.Path); err == nil {
		fileID = existing.ID
	}
	now := time.Now()
	fileMeta := &models.FileMetadata{
		ID:             fileID,
		NamespaceID:    nsID,
		Path:           strings.TrimPrefix(req.Path, "/"),
		SizeBytes:      sizeBytes,
		Checksum:       checksum,
		Tier:           models.TierHot,
		LastAccessedAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := h.fileRepo.Upsert(ctx, fileMeta); err != nil {
		internalError(c, h.logger, err)
		return
	}

	// Enqueue upload job if namespace has a cloud backend.
	if ns.CloudBackend != models.CloudBackendNone {
		job := &models.SyncJob{
			ID:          uuid.New(),
			NamespaceID: nsID,
			FileID:      fileID,
			Operation:   models.SyncOperationUpload,
			Status:      models.SyncStatusPending,
			CreatedAt:   now,
		}
		if err := h.syncJobRepo.Create(ctx, job); err == nil {
			_ = h.workerPool.Submit(job)
		}
	}
	created(c, fileMeta)
}

// List handles GET /api/v1/namespaces/:id/files.
// Supports query params: tier, cloud_synced, page, per_page.
func (h *FileHandler) List(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	page, perPage, offset := parsePage(c)
	tierFilter := c.Query("tier")
	syncedFilter := c.Query("cloud_synced") // "true" | "false" | ""

	files, err := h.fileRepo.ListByNamespace(c.Request.Context(), nsID, perPage, offset)
	if err != nil {
		internalError(c, h.logger, err)
		return
	}

	// In-memory filter for tier / cloud_synced
	// (pushing these down to DB queries is a Phase 2 optimisation).
	result := files[:0]
	for _, f := range files {
		if tierFilter != "" && string(f.Tier) != tierFilter {
			continue
		}
		if syncedFilter == "true" && !f.CloudSynced {
			continue
		}
		if syncedFilter == "false" && f.CloudSynced {
			continue
		}
		result = append(result, f)
	}
	okList(c, result, len(result), page, perPage)
}

// GetByPath handles GET /api/v1/namespaces/:id/files/*path.
func (h *FileHandler) GetByPath(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	if relPath == "" {
		badRequest(c, "invalid_path", "file path must not be empty")
		return
	}
	file, err := h.fileRepo.GetByPath(c.Request.Context(), nsID, relPath)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "file not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	// Fire-and-forget access count update.
	go func() { _ = h.fileRepo.IncrementAccess(c.Request.Context(), file.ID) }()
	ok(c, file)
}

// Delete handles DELETE /api/v1/namespaces/:id/files/*path.
// Removes file metadata from the DB and enqueues a cloud delete job if synced.
func (h *FileHandler) Delete(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	ctx := c.Request.Context()

	file, err := h.fileRepo.GetByPath(ctx, nsID, relPath)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "file not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}

	// Enqueue cloud delete job before removing from DB.
	if file.CloudSynced && file.CloudKey != "" {
		job := &models.SyncJob{
			ID:          uuid.New(),
			NamespaceID: nsID,
			FileID:      file.ID,
			Operation:   models.SyncOperationDelete,
			Status:      models.SyncStatusPending,
			CreatedAt:   time.Now(),
		}
		if err := h.syncJobRepo.Create(ctx, job); err == nil {
			_ = h.workerPool.Submit(job)
		}
	}
	c.Status(204)
}
