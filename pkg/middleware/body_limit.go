package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func BodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			abortJSON(c, http.StatusRequestEntityTooLarge, "request body too large", "PAYLOAD_TOO_LARGE")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
