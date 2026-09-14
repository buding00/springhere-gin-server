package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func AccessLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		path := c.Request.URL.Path
		status := c.Writer.Status()
		if (path == "/healthz" || path == "/readyz") && status < 500 {
			return
		}
		fields := []zap.Field{zap.String("request_id", c.GetString("request_id")), zap.String("method", c.Request.Method), zap.String("path", path), zap.Int("status", status), zap.Duration("latency", time.Since(started))}
		if status >= 500 && len(c.Errors) > 0 {
			fields = append(fields, zap.Error(c.Errors.Last().Err))
		}
		log.Info("http request", fields...)
	}
}
