package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/fast-template/springhere-gin-server/internal/data"
	"github.com/fast-template/springhere-gin-server/internal/entity"
	"github.com/fast-template/springhere-gin-server/internal/model"
	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/fast-template/springhere-gin-server/pkg/middleware"
	"github.com/fast-template/springhere-gin-server/pkg/response"
	"github.com/fast-template/springhere-gin-server/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AuthService 处理登录、刷新、退出；登录态以 Redis 为准。
type AuthService struct {
	repo              *data.UserRepository
	sessions          *data.RedisAuthSessionStore
	captchas          *data.RedisCaptchaStore
	images            *utils.ImageCaptcha
	tokens            utils.Manager
	logger            *zap.Logger
	refreshTTL        time.Duration
	secureCookie      bool
	refreshCookieName string
	accessTTL         time.Duration
	captchaConfig     config.CaptchaConfig
}

// NewAuthService 创建认证服务。
func NewAuthService(repo *data.UserRepository, sessions *data.RedisAuthSessionStore, captchas *data.RedisCaptchaStore, images *utils.ImageCaptcha, tokens utils.Manager, cfg config.AuthConfig, captcha config.CaptchaConfig, logger *zap.Logger) *AuthService {
	return &AuthService{
		repo:              repo,
		sessions:          sessions,
		captchas:          captchas,
		images:            images,
		tokens:            tokens,
		logger:            logger,
		refreshTTL:        cfg.RefreshTokenTTL,
		secureCookie:      cfg.RefreshCookieSecure,
		refreshCookieName: cfg.RefreshCookieName,
		accessTTL:         cfg.AccessTokenTTL,
		captchaConfig:     captcha,
	}
}

// Me 返回当前登录用户的公开资料。
func (s *AuthService) Me(c *gin.Context) {
	principal, ok := middleware.CurrentPrincipal(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("authentication required", "UNAUTHORIZED"))
		return
	}
	c.JSON(http.StatusOK, response.OKWithData(entity.AuthUser{
		ID:     principal.ID,
		Email:  principal.Email,
		Remark: principal.Remark,
		Role:   principal.Role,
	}))
}

// InvalidateUserSessions 删除某用户全部 Redis 登录态。
func (s *AuthService) InvalidateUserSessions(ctx context.Context, userID string) error {
	return s.sessions.DeleteUserSessions(ctx, userID)
}

// UsersOnline 批量查询用户是否仍有未过期登录会话。
func (s *AuthService) UsersOnline(ctx context.Context, userIDs []string) (map[string]bool, error) {
	return s.sessions.OnlineMap(ctx, userIDs)
}

// Captcha 生成一次性图形验证码。
func (s *AuthService) Captcha(c *gin.Context) {
	allowed, err := s.captchas.AllowIssue(c.Request.Context(), c.ClientIP(), s.captchaConfig.IssueLimitPerMinute, time.Minute)
	if err != nil {
		s.logger.Error("rate limit captcha issue", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return
	}
	if !allowed {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, response.FailWithCode("too many requests", "TOO_MANY_REQUESTS"))
		return
	}
	id, image, answer, err := s.images.Generate()
	if err != nil {
		s.logger.Error("generate captcha", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if err := s.captchas.Put(c.Request.Context(), id, answer, s.captchaConfig.TTL); err != nil {
		s.logger.Error("store captcha", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return
	}
	c.JSON(http.StatusOK, response.OKWithData(entity.CaptchaChallenge{
		CaptchaID: id,
		Image:     image,
		ExpiresIn: int(s.captchaConfig.TTL.Seconds()),
	}))
}

// Login 校验账号密码，签发 Access Token 并写入 Refresh Cookie。
func (s *AuthService) Login(c *gin.Context) {
	var req entity.LoginCommand
	if err := c.ShouldBindJSON(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, response.FailWithCode("request body too large", "PAYLOAD_TOO_LARGE"))
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if !s.consumeLoginCaptcha(c, req.CaptchaID, req.Captcha) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" || len(req.Password) > 1024 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("invalid credentials", "INVALID_CREDENTIALS"))
		return
	}
	user, err := s.repo.FindUserByEmail(c.Request.Context(), req.Email)
	if errors.Is(err, data.ErrNotFound) {
		utils.DummyCompare(req.Password)
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("invalid credentials", "INVALID_CREDENTIALS"))
		return
	}
	if err != nil {
		s.logger.Error("find user by email", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if !user.Active || utils.Compare(user.PasswordHash, req.Password) != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("invalid credentials", "INVALID_CREDENTIALS"))
		return
	}
	refreshToken, err := utils.Random()
	if err != nil {
		s.logger.Error("generate refresh token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	sessionID, accessJTI := newID(), newID()
	session := entity.NewAuthSession(user)
	session.AccessJTI = accessJTI
	if err := s.sessions.Create(c.Request.Context(), sessionID, session, utils.HashTokenHex(refreshToken), s.refreshTTL); err != nil {
		s.logger.Error("create authentication session", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return
	}
	accessToken, err := s.tokens.Issue(user.ID, sessionID, accessJTI)
	if err != nil {
		_ = s.sessions.Delete(c.Request.Context(), sessionID)
		s.logger.Error("issue access token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	result := entity.LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, RefreshExpiresAt: time.Now().Add(s.refreshTTL), User: user}
	s.setRefreshCookie(c, result.RefreshToken, result.RefreshExpiresAt)
	c.JSON(http.StatusOK, response.OKWithData(entity.Token{AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: int(s.accessTTL.Seconds()), User: entity.ToAuthUser(result.User)}))
}

// Refresh 轮换 Refresh Token 和 Access Token。
func (s *AuthService) Refresh(c *gin.Context) {
	rawRefresh, err := c.Cookie(s.refreshCookieName)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("authentication required", "UNAUTHORIZED"))
		return
	}
	nextRefresh, err := utils.Random()
	if err != nil {
		s.logger.Error("generate refresh token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	nextAccessJTI := newID()
	sessionID, session, remainingTTL, err := s.sessions.Rotate(c.Request.Context(), utils.HashTokenHex(rawRefresh), utils.HashTokenHex(nextRefresh), nextAccessJTI)
	if errors.Is(err, data.ErrNotFound) {
		s.clearRefreshCookie(c)
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("authentication required", "UNAUTHORIZED"))
		return
	}
	if err != nil {
		s.logger.Error("rotate authentication session", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return
	}
	if !session.Active || !session.Role.Valid() {
		_ = s.sessions.Delete(c.Request.Context(), sessionID)
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("authentication required", "UNAUTHORIZED"))
		return
	}
	accessToken, err := s.tokens.Issue(session.UserID, sessionID, nextAccessJTI)
	if err != nil {
		_ = s.sessions.Delete(c.Request.Context(), sessionID)
		s.logger.Error("issue access token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	result := entity.LoginResult{AccessToken: accessToken, RefreshToken: nextRefresh, RefreshExpiresAt: time.Now().Add(remainingTTL), User: model.User{ID: session.UserID, Email: session.Email, Remark: session.Remark, Role: session.Role, Active: session.Active}}
	s.setRefreshCookie(c, result.RefreshToken, result.RefreshExpiresAt)
	c.JSON(http.StatusOK, response.OKWithData(entity.Token{AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: int(s.accessTTL.Seconds()), User: entity.ToAuthUser(result.User)}))
}

// Logout 删除当前会话并清除 Refresh Cookie。
func (s *AuthService) Logout(c *gin.Context) {
	rawRefresh, _ := c.Cookie(s.refreshCookieName)
	if err := s.sessions.DeleteByRefreshDigest(c.Request.Context(), utils.HashTokenHex(rawRefresh)); err != nil {
		s.logger.Error("delete authentication session", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return
	}
	s.clearRefreshCookie(c)
	c.JSON(http.StatusOK, response.OKWithMessage("logged out"))
}

// consumeLoginCaptcha 在校验账号密码前核销验证码。关闭验证码时直接通过。
func (s *AuthService) consumeLoginCaptcha(c *gin.Context, captchaID, captcha string) bool {
	if !s.captchaConfig.Enabled {
		return true
	}
	captchaID = strings.TrimSpace(captchaID)
	captcha = strings.TrimSpace(captcha)
	if captchaID == "" || captcha == "" || len(captcha) > 16 {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return false
	}
	parsed, err := uuid.Parse(captchaID)
	if err != nil || parsed.String() != captchaID {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return false
	}
	ok, err := s.captchas.Consume(c.Request.Context(), captchaID, captcha)
	if err != nil {
		s.logger.Error("consume captcha", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return false
	}
	if !ok {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid captcha", "INVALID_CAPTCHA"))
		return false
	}
	return true
}

// setRefreshCookie 写入 HttpOnly Refresh Cookie。
func (s *AuthService) setRefreshCookie(c *gin.Context, value string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: s.refreshCookieName, Value: value, Path: "/api/v1/auth", HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}

// clearRefreshCookie 删除 Refresh Cookie。
func (s *AuthService) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: s.refreshCookieName, Value: "", Path: "/api/v1/auth", HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// newID 生成 UUID，用作 sessionID 或 jti。
func newID() string { return uuid.NewString() }
