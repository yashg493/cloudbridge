package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger returns a Gin middleware that emits one structured zap log line
// per request after the handler chain completes.
//
// Log fields:
//   - request_id  – X-Request-ID header value (or a generated UUID if absent)
//   - method      – HTTP method
//   - path        – URL path (without query string)
//   - status      – HTTP response status code
//   - latency_ms  – handler duration in milliseconds
//   - ip          – client remote address
//   - user_agent  – User-Agent header
//   - bytes_out   – response body size in bytes
//   - errors      – any errors attached via c.Error(...)
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// TODO: read X-Request-ID header; generate uuid if absent
		// TODO: store request ID in gin context: c.Set("request_id", reqID)
		// TODO: echo back in response: c.Header("X-Request-ID", reqID)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			// TODO: zap.String("request_id", reqID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("bytes_out", c.Writer.Size()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypeAny).String()))
		}

		switch {
		case status >= 500:
			logger.Error("request", fields...)
		case status >= 400:
			logger.Warn("request", fields...)
		default:
			logger.Info("request", fields...)
		}
	}
}
