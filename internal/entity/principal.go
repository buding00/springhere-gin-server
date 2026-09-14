package entity

import (
	"github.com/fast-template/springhere-gin-server/internal/model"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
)

// Principal 是鉴权通过后放进请求上下文的当前用户。
type Principal struct {
	ID        string        `json:"id"`
	Email     string        `json:"email"`
	Remark    string        `json:"remark"`
	Role      constant.Role `json:"role"`
	SessionID string        `json:"-"`
}

// AuthSession 是 Redis 里一份登录会话的用户快照，不含原始 Token。
type AuthSession struct {
	UserID    string        `json:"user_id"`
	Email     string        `json:"email"`
	Remark    string        `json:"remark"`
	Role      constant.Role `json:"role"`
	Active    bool          `json:"active"`
	AccessJTI string        `json:"access_jti"`
}

// NewAuthSession 用数据库用户生成会话快照。
func NewAuthSession(user model.User) AuthSession {
	return AuthSession{
		UserID: user.ID,
		Email:  user.Email,
		Remark: user.Remark,
		Role:   user.Role,
		Active: user.Active,
	}
}

// Principal 转成 handler 使用的当前用户。
func (s AuthSession) Principal(sessionID string) Principal {
	return Principal{ID: s.UserID, Email: s.Email, Remark: s.Remark, Role: s.Role, SessionID: sessionID}
}
