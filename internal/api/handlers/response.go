package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Response is the standard JSON envelope returned by every CloudBridge endpoint.
type Response struct {
	Data  interface{} `json:"data"`
	Error *ErrBody    `json:"error"`
	Meta  *Meta       `json:"meta,omitempty"`
}

// ErrBody carries a machine-readable error code and a human-readable message.
type ErrBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Meta carries pagination information for list responses.
type Meta struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// parsePage parses page/per_page query params with safe defaults.
func parsePage(c *gin.Context) (page, perPage, offset int) {
	page = max(1, queryInt(c, "page", 1))
	perPage = min(max(1, queryInt(c, "per_page", 20)), 200)
	offset = (page - 1) * perPage
	return
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// ── Response helpers ─────────────────────────────────────────────────────────

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Data: data})
}

func created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{Data: data})
}

func okList(c *gin.Context, data interface{}, total, page, perPage int) {
	c.JSON(http.StatusOK, Response{
		Data: data,
		Meta: &Meta{Total: total, Page: page, PerPage: perPage},
	})
}

func badRequest(c *gin.Context, code, message string) {
	c.JSON(http.StatusBadRequest, Response{
		Error: &ErrBody{Code: code, Message: message},
	})
}

func notFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Error: &ErrBody{Code: "not_found", Message: message},
	})
}

func conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, Response{
		Error: &ErrBody{Code: "conflict", Message: message},
	})
}

func internalError(c *gin.Context, logger *zap.Logger, err error) {
	logger.Error("internal server error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, Response{
		Error: &ErrBody{Code: "internal_error", Message: "an unexpected error occurred"},
	})
}
