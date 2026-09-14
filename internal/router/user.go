package router

import (
	"github.com/fast-template/springhere-gin-server/internal/service"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
	"github.com/fast-template/springhere-gin-server/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// UserRouter 注册管理员用户管理路由。
type UserRouter struct {
	service *service.UserService
	auth    *middleware.AuthMiddleware
}

// NewUserRouter 创建用户管理路由。
func NewUserRouter(service *service.UserService, auth *middleware.AuthMiddleware) *UserRouter {
	return &UserRouter{service: service, auth: auth}
}

// Register 挂上用户增删改查、启停和踢下线接口。
func (r *UserRouter) Register(v1 *gin.RouterGroup) {
	users := v1.Group("/users", r.auth.Required(constant.RoleAdmin))
	users.GET("", r.service.List)
	users.POST("", r.service.Create)
	users.GET("/:id", r.service.Get)
	users.PATCH("/:id", r.service.Update)
	users.DELETE("/:id", r.service.Delete)
	users.PATCH("/:id/status", r.service.SetActive)
	users.POST("/:id/kick", r.service.Kick)
}
