package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/buding00/springhere-gin-server/internal/model"
	"github.com/buding00/springhere-gin-server/internal/model/query"
	"github.com/buding00/springhere-gin-server/pkg/constant"
	"github.com/buding00/springhere-gin-server/pkg/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const bootstrapAdminEmail = "admin@localhost.com"

var (
	// ErrNotFound 表示记录不存在。
	ErrNotFound = errors.New("data: not found")
	// ErrConflict 表示唯一约束冲突，通常是邮箱已存在。
	ErrConflict = errors.New("data: conflict")
)

// UserRepository 负责 users 表的读写。
type UserRepository struct{ query *query.Query }

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{query: query.Use(db)}
}

// EnsureBootstrapAdmin 确保引导管理员存在：没有则创建，有则重置密码。
func (r *UserRepository) EnsureBootstrapAdmin(ctx context.Context) (email, password, userID string, err error) {
	password = uuid.NewString()
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return "", "", "", err
	}
	email = bootstrapAdminEmail
	now := time.Now()
	existing, err := r.query.User.WithContext(ctx).Where(r.query.User.Email.Eq(email)).Take()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		created := model.User{
			ID:           uuid.NewString(),
			Email:        email,
			Remark:       "admin",
			PasswordHash: passwordHash,
			Active:       true,
			Role:         constant.RoleAdmin,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := r.query.User.WithContext(ctx).Create(&created); err != nil {
			return "", "", "", err
		}
		return email, password, created.ID, nil
	}
	if err != nil {
		return "", "", "", err
	}
	_, err = r.query.User.WithContext(ctx).Where(r.query.User.ID.Eq(existing.ID)).UpdateSimple(
		r.query.User.PasswordHash.Value(passwordHash),
		r.query.User.Remark.Value("admin"),
		r.query.User.Role.Value(string(constant.RoleAdmin)),
		r.query.User.Active.Value(true),
	)
	if err != nil {
		return "", "", "", err
	}
	return email, password, existing.ID, nil
}

// ListUsers 分页列出用户，可按邮箱、角色、启用状态过滤。
func (r *UserRepository) ListUsers(ctx context.Context, offset, limit int, email string, role constant.Role, active *bool) ([]model.User, int64, error) {
	users := r.query.User.WithContext(ctx)
	if email != "" {
		users = users.Where(r.query.User.Email.Like("%" + escapeLikePattern(normalizeEmail(email)) + "%"))
	}
	if role != "" {
		users = users.Where(r.query.User.Role.Eq(string(role)))
	}
	if active != nil {
		users = users.Where(r.query.User.Active.Is(*active))
	}
	rows, total, err := users.Order(r.query.User.CreatedAt.Desc()).FindByPage(offset, limit)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.User, 0, len(rows))
	for _, user := range rows {
		result = append(result, *user)
	}
	return result, total, nil
}

// FindUserByID 按 ID 查找用户，找不到返回 ErrNotFound。
func (r *UserRepository) FindUserByID(ctx context.Context, id string) (model.User, error) {
	user, err := r.query.User.WithContext(ctx).Where(r.query.User.ID.Eq(id)).Take()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return *user, nil
}

// FindUserByEmail 按小写邮箱查找用户，找不到返回 ErrNotFound。
func (r *UserRepository) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	user, err := r.query.User.WithContext(ctx).Where(r.query.User.Email.Eq(normalizeEmail(email))).Take()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return *user, nil
}

// CreateUser 插入用户，邮箱冲突返回 ErrConflict。
func (r *UserRepository) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	if err := r.query.User.WithContext(ctx).Create(&user); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.User{}, ErrConflict
		}
		return model.User{}, err
	}
	return user, nil
}

// UpdateUser 更新用户资料、角色或密码。
func (r *UserRepository) UpdateUser(ctx context.Context, user model.User) (model.User, error) {
	result, err := r.query.User.WithContext(ctx).
		Where(r.query.User.ID.Eq(user.ID)).
		Select(r.query.User.Email, r.query.User.Remark, r.query.User.PasswordHash, r.query.User.Role, r.query.User.UpdatedAt).
		Updates(&user)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.User{}, ErrConflict
		}
		return model.User{}, err
	}
	if result.RowsAffected == 0 {
		return model.User{}, ErrNotFound
	}
	return r.FindUserByID(ctx, user.ID)
}

// SetUserActive 启用或禁用用户。
func (r *UserRepository) SetUserActive(ctx context.Context, id string, active bool) (model.User, error) {
	result, err := r.query.User.WithContext(ctx).
		Where(r.query.User.ID.Eq(id)).
		UpdateSimple(r.query.User.Active.Value(active))
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.User{}, ErrConflict
		}
		return model.User{}, err
	}
	if result.RowsAffected == 0 {
		return model.User{}, ErrNotFound
	}
	return r.FindUserByID(ctx, id)
}

// DeleteUser 物理删除用户。
func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.query.User.WithContext(ctx).Where(r.query.User.ID.Eq(id)).Delete()
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrConflict
		}
		return err
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// normalizeEmail 去掉首尾空白并转成小写。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// escapeLikePattern 转义 LIKE 通配符，避免把用户输入当模式。
func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
