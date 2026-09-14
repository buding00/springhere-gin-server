package data

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	captchaKeyPrefix   = "captcha:v1:"
	captchaIPKeyPrefix = "captcha:v1:ip:"
)

// consumeCaptchaScript 无论对错都删除验证码，防止重放。
var consumeCaptchaScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then return false end
redis.call('DEL', KEYS[1])
return current
`)

// issueCaptchaScript 按 IP 计数，窗口从第一次拉取开始。
var issueCaptchaScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return n
`)

// RedisCaptchaStore 用 Redis 保存一次性图形验证码。
type RedisCaptchaStore struct{ client redis.Cmdable }

// NewRedisCaptchaStore 创建验证码存储。
func NewRedisCaptchaStore(client redis.Cmdable) *RedisCaptchaStore {
	return &RedisCaptchaStore{client: client}
}

// Put 写入规范化后的验证码答案。
func (s *RedisCaptchaStore) Put(ctx context.Context, id, answer string, ttl time.Duration) error {
	if id == "" || answer == "" || ttl <= 0 {
		return fmt.Errorf("invalid captcha record")
	}
	if err := s.client.Set(ctx, captchaKey(id), answer, ttl).Err(); err != nil {
		return fmt.Errorf("store captcha: %w", err)
	}
	return nil
}

// Consume 原子核销验证码。答案不匹配、过期或不存在都返回 false。
func (s *RedisCaptchaStore) Consume(ctx context.Context, id, answer string) (bool, error) {
	if id == "" || answer == "" {
		return false, nil
	}
	stored, err := consumeCaptchaScript.Run(ctx, s.client, []string{captchaKey(id)}).Text()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume captcha: %w", err)
	}
	normalized := NormalizeCaptcha(answer)
	if len(stored) != len(normalized) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(normalized)) == 1, nil
}

// AllowIssue 限制同一 IP 在时间窗口内的拉取次数。
func (s *RedisCaptchaStore) AllowIssue(ctx context.Context, ip string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, fmt.Errorf("invalid captcha issue limit")
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		return false, fmt.Errorf("invalid captcha issue window")
	}
	n, err := issueCaptchaScript.Run(ctx, s.client, []string{captchaIPKey(ip)}, seconds).Int64()
	if err != nil {
		return false, fmt.Errorf("rate limit captcha issue: %w", err)
	}
	return n <= int64(limit), nil
}

// NormalizeCaptcha 去掉空白并把答案转为大写，便于大小写不敏感比较。
func NormalizeCaptcha(answer string) string {
	return strings.ToUpper(strings.TrimSpace(answer))
}

func captchaKey(id string) string { return captchaKeyPrefix + id }

func captchaIPKey(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	return captchaIPKeyPrefix + ip
}
