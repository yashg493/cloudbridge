package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/store"
)

// NamespaceHandler handles namespace (file share) CRUD endpoints.
type NamespaceHandler struct {
	nsRepo *store.NamespaceRepo
	logger *zap.Logger
}

// NewNamespaceHandler creates a NamespaceHandler.
func NewNamespaceHandler(nsRepo *store.NamespaceRepo, logger *zap.Logger) *NamespaceHandler {
	return &NamespaceHandler{nsRepo: nsRepo, logger: logger}
}

// Create handles POST /api/v1/namespaces
// Body: {"name": "...", "description": "...", "mount_path": "/...", "quota_bytes": N}
func (h *NamespaceHandler) Create(c *gin.Context) {
	// TODO: bind and validate request body into models.Namespace
	// TODO: required fields: name, mount_path
	// TODO: generate UUID, set status=active, timestamps
	// TODO: h.nsRepo.Create(c.Request.Context(), &ns)
	// TODO: handle unique constraint violation → 409 Conflict
	// TODO: c.JSON(http.StatusCreated, ns)
	h.logger.Warn("Namespace Create not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Get handles GET /api/v1/namespaces/:namespace_id
func (h *NamespaceHandler) Get(c *gin.Context) {
	// TODO: parse namespace_id path param (uuid.Parse)
	// TODO: h.nsRepo.GetByID(c.Request.Context(), id)
	// TODO: not found → 404; other error → 500
	// TODO: c.JSON(http.StatusOK, ns)
	h.logger.Warn("Namespace Get not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// List handles GET /api/v1/namespaces
func (h *NamespaceHandler) List(c *gin.Context) {
	// TODO: h.nsRepo.List(c.Request.Context())
	// TODO: return {"data": [...], "count": len}
	h.logger.Warn("Namespace List not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Update handles PUT /api/v1/namespaces/:namespace_id
// Supports partial update: only provided fields are changed.
func (h *NamespaceHandler) Update(c *gin.Context) {
	// TODO: parse namespace_id; bind request body
	// TODO: h.nsRepo.GetByID to confirm existence
	// TODO: apply patch fields (name, description, mount_path, quota_bytes)
	// TODO: h.nsRepo.Update(c.Request.Context(), &ns)
	// TODO: c.JSON(http.StatusOK, ns)
	h.logger.Warn("Namespace Update not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Delete handles DELETE /api/v1/namespaces/:namespace_id
// Returns 409 if the namespace still contains active files.
func (h *NamespaceHandler) Delete(c *gin.Context) {
	// TODO: parse namespace_id
	// TODO: h.nsRepo.Delete(c.Request.Context(), id)
	// TODO: map ErrNamespaceNotEmpty → 409 Conflict with descriptive message
	// TODO: c.Status(http.StatusNoContent)
	h.logger.Warn("Namespace Delete not implemented")
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
