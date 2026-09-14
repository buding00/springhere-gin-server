package router

import (
	"github.com/fast-template/springhere-gin-server/internal/service"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
	"github.com/fast-template/springhere-gin-server/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// AuthRouter 注册认证相关路由。
type AuthRouter struct {
	service        *service.AuthService
	auth           *middleware.AuthMiddleware
	allowedOrigins []string
}

// NewAuthRouter 创建认证路由。
func NewAuthRouter(service *service.AuthService, auth *middleware.AuthMiddleware, allowedOrigins []string) *AuthRouter {
	return &AuthRouter{service: service, auth: auth, allowedOrigins: allowedOrigins}
}

// Register 挂上验证码、登录、刷新、退出和当前用户接口。
func (r *AuthRouter) Register(v1 *gin.RouterGroup) {
	public := v1.Group("/auth")
	trustedOrigin := middleware.RequireAllowedOrigin(r.allowedOrigins)
	public.GET("/captcha", r.service.Captcha)
	public.POST("/login", r.service.Login)
	public.POST("/refresh", trustedOrigin, r.service.Refresh)
	public.POST("/logout", trustedOrigin, r.service.Logout)
	public.GET("/me", r.auth.Required(constant.RoleAdmin, constant.RoleUser), r.service.Me)
}
