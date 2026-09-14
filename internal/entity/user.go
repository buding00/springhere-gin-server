package entity

import (
	"time"

	"github.com/fast-template/springhere-gin-server/internal/model"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
)

// User 是用户管理接口的响应体，不含密码。
type User struct {
	ID        string        `json:"id"`
	Email     string        `json:"email"`
	Remark    string        `json:"remark"`
	Role      constant.Role `json:"role"`
	Active    bool          `json:"active"`
	Online    bool          `json:"online"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ToUser 把数据库用户转成接口响应。
func ToUser(user model.User) User {
	return User{
		ID:        user.ID,
		Email:     user.Email,
		Remark:    user.Remark,
		Role:      user.Role,
		Active:    user.Active,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// ToUsers 批量转换用户列表。
func ToUsers(users []model.User) []User {
	result := make([]User, 0, len(users))
	for _, user := range users {
		result = append(result, ToUser(user))
	}
	return result
}

// UserListQuery 是用户列表的查询参数。
type UserListQuery struct {
	Page     int           `form:"page,default=1" binding:"min=1"`
	PageSize int           `form:"page_size,default=20" binding:"min=1,max=100"`
	Email    string        `form:"email" binding:"omitempty,max=254"`
	Role     constant.Role `form:"role"`
	Active   *bool         `form:"active"`
}

// CreateUserCommand 是创建用户的请求体。
type CreateUserCommand struct {
	Email    string        `json:"email" binding:"required,email,max=254"`
	Remark   string        `json:"remark" binding:"required,max=100"`
	Password string        `json:"password" binding:"required,min=8,max=72"`
	Role     constant.Role `json:"role" binding:"required"`
	Active   *bool         `json:"active"`
}

// UpdateUserCommand 是修改用户的请求体，字段均可选。
type UpdateUserCommand struct {
	Email    *string        `json:"email" binding:"omitempty,email,max=254"`
	Remark   *string        `json:"remark" binding:"omitempty,max=100"`
	Password *string        `json:"password" binding:"omitempty,min=8,max=72"`
	Role     *constant.Role `json:"role"`
}

// Empty 表示没有要修改的字段。
func (c UpdateUserCommand) Empty() bool {
	return c.Email == nil && c.Remark == nil && c.Password == nil && c.Role == nil
}

// SetUserActiveCommand 是启用/禁用用户的请求体。
type SetUserActiveCommand struct {
	Active *bool `json:"active" binding:"required"`
}
