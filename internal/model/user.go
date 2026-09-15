package model

import (
	"time"

	"github.com/buding00/springhere-gin-server/pkg/constant"
)

// User 对应 users 表，密码哈希不得出现在 HTTP 响应里。
type User struct {
	ID           string
	Email        string
	Remark       string `gorm:"column:remark"`
	PasswordHash string `gorm:"column:password_hash"`
	Active       bool   `gorm:"column:is_active"`
	Role         constant.Role
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名为 users。
func (User) TableName() string { return "users" }
