package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/fast-template/springhere-gin-server/internal/data"
	"github.com/fast-template/springhere-gin-server/internal/entity"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
	"github.com/fast-template/springhere-gin-server/pkg/response"
	"github.com/fast-template/springhere-gin-server/pkg/utils"
	"github.com/gin-gonic/gin"
)

const principalContextKey = "principal"

type AuthSessionReader interface {
	Get(context.Context, string) (entity.AuthSession, error)
}

// AuthMiddleware owns the dependencies required to verify protected requests.
// Routers only use Required to declare which roles are allowed per route.
type AuthMiddleware struct {
	tokens   utils.Manager
	sessions AuthSessionReader
}

func NewAuthMiddleware(tokens utils.Manager, sessions AuthSessionReader) *AuthMiddleware {
	return &AuthMiddleware{tokens: tokens, sessions: sessions}
}

// Required performs JWT, Redis session, and route-role verification in a
// single middleware. Callers must explicitly list every role allowed to pass.
func (m *AuthMiddleware) Required(roles ...constant.Role) gin.HandlerFunc {
	allowed := make(map[constant.Role]struct{}, len(roles))
	for _, role := range roles {
		if !role.Valid() {
			panic(fmt.Sprintf("invalid authorization role %q", role))
		}
		allowed[role] = struct{}{}
	}
	if len(allowed) == 0 {
		panic("at least one authorization role is required")
	}
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || parts[0] != "Bearer" {
			abortJSON(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
			return
		}
		identity, err := m.tokens.Parse(parts[1])
		if err != nil {
			abortJSON(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
			return
		}
		session, err := m.sessions.Get(c.Request.Context(), identity.SessionID)
		if errors.Is(err, data.ErrNotFound) {
			abortJSON(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
			return
		}
		if err != nil {
			_ = c.Error(err)
			abortJSON(c, http.StatusServiceUnavailable, "service temporarily unavailable", "SERVICE_UNAVAILABLE")
			return
		}
		if !session.Active || !session.Role.Valid() || session.UserID != identity.UserID || session.AccessJTI != identity.AccessJTI {
			abortJSON(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
			return
		}
		if _, ok := allowed[session.Role]; !ok {
			abortJSON(c, http.StatusForbidden, "permission denied", "FORBIDDEN")
			return
		}
		principal := session.Principal(identity.SessionID)
		c.Set(principalContextKey, principal)
		c.Next()
	}
}

// CurrentPrincipal returns the authenticated user snapshot from Gin context.
func CurrentPrincipal(c *gin.Context) (entity.Principal, bool) {
	value, ok := c.Get(principalContextKey)
	if !ok {
		return entity.Principal{}, false
	}
	principal, ok := value.(entity.Principal)
	return principal, ok
}

func abortJSON(c *gin.Context, status int, message, code string) {
	c.AbortWithStatusJSON(status, response.FailWithCode(message, code))
}
