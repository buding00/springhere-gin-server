package service

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/buding00/springhere-gin-server/internal/data"
	"github.com/buding00/springhere-gin-server/internal/entity"
	"github.com/buding00/springhere-gin-server/internal/model"
	"github.com/buding00/springhere-gin-server/pkg/middleware"
	"github.com/buding00/springhere-gin-server/pkg/response"
	"github.com/buding00/springhere-gin-server/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserService 处理管理员用户管理接口。
type UserService struct {
	repo   *data.UserRepository
	auth   *AuthService
	logger *zap.Logger
}

// NewUserService 创建用户管理服务。
func NewUserService(repo *data.UserRepository, auth *AuthService, logger *zap.Logger) *UserService {
	return &UserService{repo: repo, auth: auth, logger: logger}
}

// List 分页查询用户。
func (s *UserService) List(c *gin.Context) {
	params := entity.UserListQuery{Page: 1, PageSize: 20}
	if err := c.ShouldBindQuery(&params); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if params.Role != "" && !params.Role.Valid() {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	users, total, err := s.repo.ListUsers(
		c.Request.Context(),
		(params.Page-1)*params.PageSize,
		params.PageSize,
		strings.TrimSpace(params.Email),
		params.Role,
		params.Active,
	)
	if err != nil {
		s.logger.Error("list users", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	items, ok := s.withOnline(c, entity.ToUsers(users))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, response.OKWithPageData(items, int(total), params.Page, params.PageSize))
}

// Create 创建用户，密码只存 bcrypt hash。
func (s *UserService) Create(c *gin.Context) {
	var command entity.CreateUserCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, response.FailWithCode("request body too large", "PAYLOAD_TOO_LARGE"))
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if !command.Role.Valid() {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	command.Email = strings.ToLower(strings.TrimSpace(command.Email))
	command.Remark = strings.TrimSpace(command.Remark)
	if command.Email == "" || command.Remark == "" || len(command.Password) < 8 || len(command.Password) > 72 {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	passwordHash, err := utils.HashPassword(command.Password)
	if err != nil {
		s.logger.Error("hash user password", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	active := true
	if command.Active != nil {
		active = *command.Active
	}
	now := time.Now()
	user, err := s.repo.CreateUser(c.Request.Context(), model.User{
		ID:           uuid.NewString(),
		Email:        command.Email,
		Remark:       command.Remark,
		PasswordHash: passwordHash,
		Active:       active,
		Role:         command.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		if errors.Is(err, data.ErrConflict) {
			c.AbortWithStatusJSON(http.StatusConflict, response.FailWithCode("resource already exists", "CONFLICT"))
			return
		}
		s.logger.Error("create user", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	c.JSON(http.StatusCreated, response.OKWithData(entity.ToUser(user)))
}

// Get 按 ID 返回用户详情。
func (s *UserService) Get(c *gin.Context) {
	userID, ok := userIDParam(c)
	if !ok {
		return
	}
	user, err := s.repo.FindUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("get user", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	item, ok := s.withOnline(c, []entity.User{entity.ToUser(user)})
	if !ok {
		return
	}
	c.JSON(http.StatusOK, response.OKWithData(item[0]))
}

// Update 修改用户资料、角色或密码，并撤销其登录态。
func (s *UserService) Update(c *gin.Context) {
	userID, ok := userIDParam(c)
	if !ok {
		return
	}
	var command entity.UpdateUserCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, response.FailWithCode("request body too large", "PAYLOAD_TOO_LARGE"))
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if command.Empty() || (command.Role != nil && !command.Role.Valid()) {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	user, err := s.repo.FindUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("get user for update", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if command.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*command.Email))
		if email == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
			return
		}
		user.Email = email
	}
	if command.Remark != nil {
		remark := strings.TrimSpace(*command.Remark)
		if remark == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
			return
		}
		user.Remark = remark
	}
	if command.Password != nil {
		if len(*command.Password) < 8 || len(*command.Password) > 72 {
			c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
			return
		}
		user.PasswordHash, err = utils.HashPassword(*command.Password)
		if err != nil {
			s.logger.Error("hash user password", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
			return
		}
	}
	if command.Role != nil && *command.Role != user.Role {
		if rejectSelfTarget(c, userID) {
			return
		}
		user.Role = *command.Role
	}
	if !s.invalidateSessions(c, userID) {
		return
	}
	user.UpdatedAt = time.Now()
	user, err = s.repo.UpdateUser(c.Request.Context(), user)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		if errors.Is(err, data.ErrConflict) {
			c.AbortWithStatusJSON(http.StatusConflict, response.FailWithCode("resource already exists", "CONFLICT"))
			return
		}
		s.logger.Error("update user", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	c.JSON(http.StatusOK, response.OKWithData(entity.ToUser(user)))
}

// Delete 物理删除用户，不能删自己。
func (s *UserService) Delete(c *gin.Context) {
	userID, ok := userIDParam(c)
	if !ok {
		return
	}
	if _, err := s.repo.FindUserByID(c.Request.Context(), userID); err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("get user for deletion", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if rejectSelfTarget(c, userID) {
		return
	}
	if !s.invalidateSessions(c, userID) {
		return
	}
	if err := s.repo.DeleteUser(c.Request.Context(), userID); err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("delete user", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	c.JSON(http.StatusOK, response.OKWithMessage("user deleted"))
}

// SetActive 启用或禁用用户，不能操作自己。
func (s *UserService) SetActive(c *gin.Context) {
	userID, ok := userIDParam(c)
	if !ok {
		return
	}
	var command entity.SetUserActiveCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, response.FailWithCode("request body too large", "PAYLOAD_TOO_LARGE"))
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if command.Active == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return
	}
	if _, err := s.repo.FindUserByID(c.Request.Context(), userID); err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("get user for status update", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if rejectSelfTarget(c, userID) {
		return
	}
	if !s.invalidateSessions(c, userID) {
		return
	}
	user, err := s.repo.SetUserActive(c.Request.Context(), userID, *command.Active)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("set user active state", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	c.JSON(http.StatusOK, response.OKWithData(entity.ToUser(user)))
}

// Kick 踢用户下线，只清 Redis 会话。
func (s *UserService) Kick(c *gin.Context) {
	userID, ok := userIDParam(c)
	if !ok {
		return
	}
	if _, err := s.repo.FindUserByID(c.Request.Context(), userID); err != nil {
		if errors.Is(err, data.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, response.FailWithCode("resource not found", "NOT_FOUND"))
			return
		}
		s.logger.Error("get user for session invalidation", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.FailWithCode("internal server error", "INTERNAL_ERROR"))
		return
	}
	if !s.invalidateSessions(c, userID) {
		return
	}
	c.JSON(http.StatusOK, response.OKWithMessage("user sessions invalidated"))
}

// withOnline 为本页用户填充 online；Redis 失败时已写 503。
func (s *UserService) withOnline(c *gin.Context, items []entity.User) ([]entity.User, bool) {
	if len(items) == 0 {
		return items, true
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	online, err := s.auth.UsersOnline(c.Request.Context(), ids)
	if err != nil {
		s.logger.Error("load user online state", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return nil, false
	}
	for i := range items {
		items[i].Online = online[items[i].ID]
	}
	return items, true
}

// invalidateSessions 撤销目标用户全部登录；失败时已写 503。
func (s *UserService) invalidateSessions(c *gin.Context, userID string) bool {
	if err := s.auth.InvalidateUserSessions(c.Request.Context(), userID); err != nil {
		s.logger.Error("invalidate user sessions", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithCode("service temporarily unavailable", "SERVICE_UNAVAILABLE"))
		return false
	}
	return true
}

// userIDParam 读取路径中的用户 ID，空值视为非法请求。
func userIDParam(c *gin.Context) (string, bool) {
	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return "", false
	}
	return userID, true
}

// rejectSelfTarget 禁止管理员对自己执行删除、停用或改角色。
func rejectSelfTarget(c *gin.Context, userID string) bool {
	principal, ok := middleware.CurrentPrincipal(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, response.FailWithCode("authentication required", "UNAUTHORIZED"))
		return true
	}
	if principal.ID == userID {
		c.AbortWithStatusJSON(http.StatusBadRequest, response.FailWithCode("invalid request", "INVALID_REQUEST"))
		return true
	}
	return false
}
