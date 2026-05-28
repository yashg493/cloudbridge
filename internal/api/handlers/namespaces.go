package handlers

import (
	"errors"
	"fmt"
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

// NamespaceHandler handles namespace CRUD and sync endpoints.
type NamespaceHandler struct {
	nsRepo      *store.NamespaceRepo
	fileRepo    *store.FileRepo
	syncJobRepo *store.SyncJobRepo
	nfsSim      *nfs.Simulator
	workerPool  *worker.WorkerPool
	logger      *zap.Logger
}

// NewNamespaceHandler creates a NamespaceHandler.
func NewNamespaceHandler(
	nsRepo *store.NamespaceRepo,
	fileRepo *store.FileRepo,
	syncJobRepo *store.SyncJobRepo,
	nfsSim *nfs.Simulator,
	workerPool *worker.WorkerPool,
	logger *zap.Logger,
) *NamespaceHandler {
	return &NamespaceHandler{
		nsRepo:      nsRepo,
		fileRepo:    fileRepo,
		syncJobRepo: syncJobRepo,
		nfsSim:      nfsSim,
		workerPool:  workerPool,
		logger:      logger,
	}
}

// createNamespaceReq is the request body for POST /api/v1/namespaces.
type createNamespaceReq struct {
	Name            string `json:"name"         binding:"required"`
	Protocol        string `json:"protocol"     binding:"required"`
	SourcePath      string `json:"source_path"  binding:"required"`
	CloudBackend    string `json:"cloud_backend"`
	CloudBucket     string `json:"cloud_bucket"`
	CloudPrefix     string `json:"cloud_prefix"`
	ReplicationMode string `json:"replication_mode"`
	NodeCount       int    `json:"node_count"`
}

// Create handles POST /api/v1/namespaces.
func (h *NamespaceHandler) Create(c *gin.Context) {
	var req createNamespaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid_body", err.Error())
		return
	}
	if req.Protocol != "nfs" && req.Protocol != "smb" {
		badRequest(c, "invalid_protocol", "protocol must be 'nfs' or 'smb'")
		return
	}

	ctx := c.Request.Context()

	// Uniqueness check.
	existing, err := h.nsRepo.GetByName(ctx, req.Name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(c, h.logger, err)
		return
	}
	if existing != nil {
		conflict(c, fmt.Sprintf("namespace %q already exists", req.Name))
		return
	}

	cb := models.CloudBackendNone
	if req.CloudBackend != "" {
		cb = models.CloudBackend(req.CloudBackend)
	}
	rm := models.ReplicationModeAsync
	if req.ReplicationMode != "" {
		rm = models.ReplicationMode(req.ReplicationMode)
	}
	now := time.Now()
	ns := &models.Namespace{
		ID:              uuid.New(),
		Name:            req.Name,
		Protocol:        models.Protocol(req.Protocol),
		SourcePath:      req.SourcePath,
		CloudBackend:    cb,
		CloudBucket:     req.CloudBucket,
		CloudPrefix:     req.CloudPrefix,
		ReplicationMode: rm,
		Status:          models.NamespaceStatusActive,
		NodeCount:       max(1, req.NodeCount),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.nsRepo.Create(ctx, ns); err != nil {
		internalError(c, h.logger, err)
		return
	}

	// Bootstrap the NFS directory for this namespace.
	if h.nfsSim != nil {
		if wErr := h.nfsSim.WriteFile(ns.Name+"/.cloudbridge-init", []byte("CloudBridge namespace")); wErr != nil {
			h.logger.Warn("failed to create NFS namespace directory",
				zap.String("name", ns.Name), zap.Error(wErr))
		}
	}
	created(c, ns)
}

// List handles GET /api/v1/namespaces.
func (h *NamespaceHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	page, perPage, offset := parsePage(c)

	items, total, err := h.nsRepo.List(ctx, status, perPage, offset)
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	okList(c, items, total, page, perPage)
}

// Get handles GET /api/v1/namespaces/:id.
func (h *NamespaceHandler) Get(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	ns, err := h.nsRepo.GetByID(c.Request.Context(), nsID)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "namespace not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}
	ok(c, ns)
}

// Delete handles DELETE /api/v1/namespaces/:id.
func (h *NamespaceHandler) Delete(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	if err := h.nsRepo.Delete(c.Request.Context(), nsID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(c, "namespace not found")
			return
		}
		internalError(c, h.logger, err)
		return
	}
	c.Status(204)
}

// Stats handles GET /api/v1/namespaces/:id/stats.
func (h *NamespaceHandler) Stats(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
		return
	}
	ctx := c.Request.Context()

	_, err = h.nsRepo.GetByID(ctx, nsID)
	if errors.Is(err, store.ErrNotFound) {
		notFound(c, "namespace not found")
		return
	}
	if err != nil {
		internalError(c, h.logger, err)
		return
	}

	fileCount, _ := h.fileRepo.CountByNamespace(ctx, nsID)
	totalSize, _ := h.fileRepo.TotalSizeByNamespace(ctx, nsID)
	jobCounts, _ := h.syncJobRepo.CountByStatus(ctx)

	ok(c, gin.H{
		"namespace_id":     nsID,
		"file_count":       fileCount,
		"total_size_bytes": totalSize,
		"jobs_pending":     jobCounts["pending"] + jobCounts["queued"],
		"jobs_running":     jobCounts["running"],
		"jobs_failed":      jobCounts["failed"],
	})
}

// Sync handles POST /api/v1/namespaces/:id/sync.
// Lists all files in the namespace's NFS directory, upserts them in the DB,
// and enqueues an upload SyncJob for each file not yet cloud-synced.
func (h *NamespaceHandler) Sync(c *gin.Context) {
	nsID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "invalid_id", "namespace id must be a valid UUID")
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

	if h.nfsSim == nil {
		ok(c, gin.H{"jobs_created": 0, "message": "NFS simulator not configured"})
		return
	}

	nfsFiles, err := h.nfsSim.ListFiles(ns.Name)
	if err != nil {
		h.logger.Warn("sync: failed to list NFS directory",
			zap.String("ns", ns.Name), zap.Error(err))
		ok(c, gin.H{"jobs_created": 0, "files_scanned": 0})
		return
	}

	var jobsCreated int
	for _, f := range nfsFiles {
		// relPath is relative to the namespace root (strip "<ns.Name>/" prefix).
		relPath := strings.TrimPrefix(f.Path, ns.Name+"/")
		if relPath == "" || strings.HasPrefix(relPath, ".cloudbridge") {
			continue
		}

		info, err := h.nfsSim.Stat(f.Path)
		if err != nil {
			h.logger.Warn("sync: stat failed", zap.String("path", f.Path), zap.Error(err))
			continue
		}

		// Get-or-assign a stable UUID for this file.
		fileID := uuid.New()
		if existing, err := h.fileRepo.GetByPath(ctx, nsID, relPath); err == nil {
			fileID = existing.ID
		}

		now := time.Now()
		fileMeta := &models.FileMetadata{
			ID:             fileID,
			NamespaceID:    nsID,
			Path:           relPath,
			SizeBytes:      info.SizeBytes,
			Checksum:       info.Checksum,
			Tier:           models.TierHot,
			LastAccessedAt: info.ModTime,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := h.fileRepo.Upsert(ctx, fileMeta); err != nil {
			h.logger.Warn("sync: upsert failed", zap.String("path", relPath), zap.Error(err))
			continue
		}

		// Only enqueue upload if the namespace has a cloud backend.
		if ns.CloudBackend == models.CloudBackendNone {
			continue
		}
		job := &models.SyncJob{
			ID:          uuid.New(),
			NamespaceID: nsID,
			FileID:      fileID,
			Operation:   models.SyncOperationUpload,
			Status:      models.SyncStatusPending,
			CreatedAt:   now,
		}
		if err := h.syncJobRepo.Create(ctx, job); err != nil {
			h.logger.Warn("sync: create job failed", zap.Error(err))
			continue
		}
		_ = h.workerPool.Submit(job)
		jobsCreated++
	}

	ok(c, gin.H{"jobs_created": jobsCreated, "files_scanned": len(nfsFiles)})
}
