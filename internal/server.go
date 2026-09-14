package internal

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/fast-template/springhere-gin-server/pkg/middleware"
	"github.com/fast-template/springhere-gin-server/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server 持有 Gin 引擎和 HTTP 服务。
type Server struct {
	Engine          *gin.Engine
	HTTP            *http.Server
	shutdownTimeout time.Duration
	logger          *zap.Logger
}

// ReadinessChecker 检查依赖是否可用，供 /readyz 调用。
type ReadinessChecker func(context.Context) error

// NewServer 组装引擎、全局中间件、健康检查和业务路由。
func NewServer(
	cfg config.ServerConfig,
	logger *zap.Logger,
	allowedOrigins []string,
	readiness ReadinessChecker,
	registerRoutes func(*gin.RouterGroup),
) *Server {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.AccessLogger(logger), middleware.Recovery(logger), middleware.CORS(allowedOrigins), middleware.BodyLimit(cfg.MaxRequestBodyBytes))
	r.NoRoute(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("route not found", "ROUTE_NOT_FOUND"))
	})
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusMethodNotAllowed, response.FailWithCode("method not allowed", "METHOD_NOT_ALLOWED"))
	})
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, response.OKWithMessage("ok"))
	})
	r.GET("/readyz", func(c *gin.Context) {
		if err := readiness(c.Request.Context()); err != nil {
			logger.Error("readiness check failed", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
			return
		}
		c.JSON(http.StatusOK, response.OKWithMessage("ready"))
	})
	registerRoutes(r.Group("/api/v1"))
	return &Server{
		Engine:          r,
		HTTP:            &http.Server{Addr: cfg.Addr, Handler: r, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, MaxHeaderBytes: cfg.MaxHeaderBytes},
		shutdownTimeout: cfg.ShutdownTimeout,
		logger:          logger,
	}
}

// Run 开始监听，收到 SIGINT/SIGTERM 后优雅关闭。
func (s *Server) Run() error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- s.HTTP.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case <-signals:
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http server stopped unexpectedly", zap.Error(err))
		}
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.HTTP.Shutdown(ctx); err != nil {
		s.logger.Error("http server shutdown failed", zap.Error(err))
		return err
	}
	return nil
}
