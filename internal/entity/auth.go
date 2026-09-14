package entity

import (
	"time"

	"github.com/fast-template/springhere-gin-server/internal/model"
	"github.com/fast-template/springhere-gin-server/pkg/constant"
)

// LoginCommand 是登录请求体。
type LoginCommand struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required"`
	CaptchaID string `json:"captcha_id"`
	Captcha   string `json:"captcha"`
}

// CaptchaChallenge 是获取验证码接口的响应。
type CaptchaChallenge struct {
	CaptchaID string `json:"captcha_id"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expires_in"`
}

// AuthUser 是认证接口返回的公开用户信息。
type AuthUser struct {
	ID     string        `json:"id"`
	Email  string        `json:"email"`
	Remark string        `json:"remark"`
	Role   constant.Role `json:"role"`
}

// Token 是登录/刷新成功后的响应数据。
type Token struct {
	AccessToken string   `json:"access_token"`
	TokenType   string   `json:"token_type"`
	ExpiresIn   int      `json:"expires_in"`
	User        AuthUser `json:"user"`
}

// LoginResult 是登录结果；RefreshToken 只用来写 Cookie，不进 JSON。
type LoginResult struct {
	AccessToken      string
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             model.User
}

// ToAuthUser 把数据库用户转成认证响应里的公开字段。
func ToAuthUser(user model.User) AuthUser {
	return AuthUser{ID: user.ID, Email: user.Email, Remark: user.Remark, Role: user.Role}
}
