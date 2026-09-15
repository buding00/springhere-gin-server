package app

import (
	"context"
	"fmt"
	"time"

	server "github.com/buding00/springhere-gin-server/internal"
	"github.com/buding00/springhere-gin-server/internal/data"
	"github.com/buding00/springhere-gin-server/internal/router"
	"github.com/buding00/springhere-gin-server/internal/service"
	"github.com/buding00/springhere-gin-server/pkg/component"
	"github.com/buding00/springhere-gin-server/pkg/config"
	"github.com/buding00/springhere-gin-server/pkg/middleware"
	"github.com/buding00/springhere-gin-server/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type App struct {
	server *server.Server
}

func New(path string) (*App, func()) {
	cfg := config.New(path)
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	log, loggerCleanup := component.NewLogger(cfg.Logger)
	db, databaseCleanup := component.OpenPostgres(cfg.Database)
	redisClient, redisCleanup := component.NewRedis(cfg.Redis)
	userRepo := data.NewUserRepository(db)
	authSessionStore := data.NewRedisAuthSessionStore(redisClient)
	captchaStore := data.NewRedisCaptchaStore(redisClient)
	imageCaptcha := utils.NewImageCaptcha(cfg.Captcha)
	jwtManager := utils.NewJWTManager(cfg.Auth)
	authMiddleware := middleware.NewAuthMiddleware(jwtManager, authSessionStore)
	authService := service.NewAuthService(userRepo, authSessionStore, captchaStore, imageCaptcha, jwtManager, cfg.Auth, cfg.Captcha, log)
	ensureContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	email, password, userID, err := userRepo.EnsureBootstrapAdmin(ensureContext)
	cancel()
	if err != nil {
		panic(fmt.Errorf("ensure bootstrap admin: %w; create tables first: go run ./cmd/migrate -action up", err))
	}
	if err := authService.InvalidateUserSessions(context.Background(), userID); err != nil {
		panic(fmt.Errorf("invalidate bootstrap admin sessions: %w", err))
	}
	log.Info("bootstrap admin ready", zap.String("email", email), zap.String("password", password))
	userService := service.NewUserService(userRepo, authService, log)
	registerRoutes := func(v1 *gin.RouterGroup) {
		router.NewAuthRouter(authService, authMiddleware, cfg.Auth.AllowedOrigins).Register(v1)
		router.NewUserRouter(userService, authMiddleware).Register(v1)
	}
	readiness := func(ctx context.Context) error {
		checkContext, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := component.PingPostgres(checkContext, db); err != nil {
			return err
		}
		if err := component.PingRedis(checkContext, redisClient); err != nil {
			return err
		}
		return nil
	}
	httpServer := server.NewServer(cfg.Server, log, cfg.Auth.AllowedOrigins, readiness, registerRoutes)
	return &App{server: httpServer}, func() {
		redisCleanup()
		databaseCleanup()
		loggerCleanup()
	}
}
func (a *App) Run() error { return a.server.Run() }
