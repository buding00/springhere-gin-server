package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("panic recovered", zap.Any("panic", recovered), zap.Stack("stack"), zap.String("request_id", c.GetString("request_id")))
				if !c.Writer.Written() {
					abortJSON(c, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
				}
			}
		}()
		c.Next()
	}
}
