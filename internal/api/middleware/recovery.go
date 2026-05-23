package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery returns a Gin middleware that catches any panic in the handler chain,
// logs it with a full stack trace, and returns a sanitised 500 to the client.
//
// NOTE: panics should never be used for normal control flow in CloudBridge;
// this middleware is strictly a last-resort safety net.
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()

				logger.Error("recovered from panic",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("panic", fmt.Sprintf("%v", r)),
					zap.ByteString("stack", stack),
				)

				// Do NOT leak the panic value or stack trace to the client.
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "an unexpected error occurred",
				})
			}
		}()
		c.Next()
	}
}
