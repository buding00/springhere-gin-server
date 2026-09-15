package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/buding00/springhere-gin-server/internal/entity"
	"github.com/redis/go-redis/v9"
)

const (
	authSessionKeyPrefix      = "auth:v2:session:"
	authRefreshKeyPrefix      = "auth:v2:refresh:"
	authUserSessionsKeyPrefix = "auth:v2:user-sessions:"
)

// authSessionRecord 是 Redis 里一份登录会话的 JSON。
type authSessionRecord struct {
	entity.AuthSession
	RefreshDigest string `json:"refresh_digest"`
}

// RedisAuthSessionStore 用 Redis 保存当前登录态，不存原始 Token。
type RedisAuthSessionStore struct{ client redis.Cmdable }

// NewRedisAuthSessionStore 创建 Redis 登录态存储。
func NewRedisAuthSessionStore(client redis.Cmdable) *RedisAuthSessionStore {
	return &RedisAuthSessionStore{client: client}
}

// Create 写入一份新会话及其 Refresh 索引。
func (s *RedisAuthSessionStore) Create(ctx context.Context, sessionID string, session entity.AuthSession, refreshDigest string, ttl time.Duration) error {
	if sessionID == "" || session.UserID == "" || session.AccessJTI == "" || refreshDigest == "" || ttl <= 0 || !session.Role.Valid() {
		return fmt.Errorf("invalid authentication session")
	}
	recordJSON, err := encodeAuthSessionRecord(authSessionRecord{AuthSession: session, RefreshDigest: refreshDigest})
	if err != nil {
		return err
	}
	now := time.Now()
	userSessionsKey := authUserSessionsKey(session.UserID)
	if _, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, authSessionKey(sessionID), recordJSON, ttl)
		pipe.Set(ctx, authRefreshKey(refreshDigest), sessionID, ttl)
		pipe.ZRemRangeByScore(ctx, userSessionsKey, "-inf", strconv.FormatInt(now.UnixMilli(), 10))
		pipe.ZAdd(ctx, userSessionsKey, redis.Z{Score: float64(now.Add(ttl).UnixMilli()), Member: sessionID})
		// Keep the longest remaining index TTL. Expire alone would shorten it
		// when a later login uses a smaller configured refresh lifetime.
		pipe.ExpireNX(ctx, userSessionsKey, ttl)
		pipe.ExpireGT(ctx, userSessionsKey, ttl)
		return nil
	}); err != nil {
		return fmt.Errorf("store authentication session: %w", err)
	}
	return nil
}

// Get 按 sessionID 读取当前会话。
func (s *RedisAuthSessionStore) Get(ctx context.Context, sessionID string) (entity.AuthSession, error) {
	record, _, err := s.getRecord(ctx, sessionID)
	return record.AuthSession, err
}

// Rotate 原子替换 Refresh digest 和 Access jti，不延长会话 TTL。
func (s *RedisAuthSessionStore) Rotate(ctx context.Context, refreshDigest, nextRefreshDigest, nextAccessJTI string) (string, entity.AuthSession, time.Duration, error) {
	if refreshDigest == "" || nextRefreshDigest == "" || nextAccessJTI == "" {
		return "", entity.AuthSession{}, 0, fmt.Errorf("invalid refresh token rotation")
	}
	sessionID, err := s.client.Get(ctx, authRefreshKey(refreshDigest)).Result()
	if errors.Is(err, redis.Nil) {
		return "", entity.AuthSession{}, 0, ErrNotFound
	}
	if err != nil {
		return "", entity.AuthSession{}, 0, fmt.Errorf("load refresh token session: %w", err)
	}
	record, recordJSON, err := s.getRecord(ctx, sessionID)
	if err != nil {
		return "", entity.AuthSession{}, 0, err
	}
	if record.RefreshDigest != refreshDigest {
		return "", entity.AuthSession{}, 0, ErrNotFound
	}
	record.RefreshDigest = nextRefreshDigest
	record.AccessJTI = nextAccessJTI
	nextRecordJSON, err := encodeAuthSessionRecord(record)
	if err != nil {
		return "", entity.AuthSession{}, 0, err
	}
	remainingMillis, err := rotateSessionScript.Run(ctx, s.client, []string{authSessionKey(sessionID), authRefreshKey(refreshDigest), authRefreshKey(nextRefreshDigest)}, recordJSON, nextRecordJSON, sessionID).Int64()
	if err != nil {
		return "", entity.AuthSession{}, 0, fmt.Errorf("rotate authentication session: %w", err)
	}
	if remainingMillis <= 0 {
		return "", entity.AuthSession{}, 0, ErrNotFound
	}
	return sessionID, record.AuthSession, time.Duration(remainingMillis) * time.Millisecond, nil
}

// Delete 删除一份会话及其反向索引。
func (s *RedisAuthSessionStore) Delete(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := deleteSessionScript.Run(ctx, s.client, []string{authSessionKey(sessionID)}, sessionID, authRefreshKeyPrefix, authUserSessionsKeyPrefix).Err(); err != nil {
		return fmt.Errorf("delete authentication session: %w", err)
	}
	return nil
}

// DeleteByRefreshDigest 按 Refresh digest 删除对应会话。
func (s *RedisAuthSessionStore) DeleteByRefreshDigest(ctx context.Context, refreshDigest string) error {
	if refreshDigest == "" {
		return nil
	}
	sessionID, err := s.client.Get(ctx, authRefreshKey(refreshDigest)).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load refresh token session for deletion: %w", err)
	}
	return s.Delete(ctx, sessionID)
}

// DeleteUserSessions 删除某用户的全部当前会话，用于踢人。
func (s *RedisAuthSessionStore) DeleteUserSessions(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	if err := deleteUserSessionsScript.Run(ctx, s.client, []string{authUserSessionsKey(userID)}, authSessionKeyPrefix, authRefreshKeyPrefix).Err(); err != nil {
		return fmt.Errorf("delete user authentication sessions: %w", err)
	}
	return nil
}

// OnlineMap 返回每个用户是否至少有一条未过期且会话本体仍在的登录。
func (s *RedisAuthSessionStore) OnlineMap(ctx context.Context, userIDs []string) (map[string]bool, error) {
	online := make(map[string]bool, len(userIDs))
	ids := uniqueUserIDs(userIDs)
	if len(ids) == 0 {
		return online, nil
	}
	for _, id := range ids {
		online[id] = false
	}
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	rangeCmds := make([]*redis.StringSliceCmd, len(ids))
	if _, err := s.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, id := range ids {
			rangeCmds[i] = pipe.ZRangeByScore(ctx, authUserSessionsKey(id), &redis.ZRangeBy{Min: now, Max: "+inf", Offset: 0, Count: 1})
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}
	sessionIDs := make([]string, 0, len(ids))
	sessionUser := make(map[string]string, len(ids))
	for i, id := range ids {
		members, err := rangeCmds[i].Result()
		if err != nil {
			return nil, fmt.Errorf("list user sessions: %w", err)
		}
		if len(members) == 0 || members[0] == "" {
			continue
		}
		sessionIDs = append(sessionIDs, members[0])
		sessionUser[members[0]] = id
	}
	if len(sessionIDs) == 0 {
		return online, nil
	}
	existsCmds := make([]*redis.IntCmd, len(sessionIDs))
	if _, err := s.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, sessionID := range sessionIDs {
			existsCmds[i] = pipe.Exists(ctx, authSessionKey(sessionID))
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("check user sessions: %w", err)
	}
	for i, sessionID := range sessionIDs {
		n, err := existsCmds[i].Result()
		if err != nil {
			return nil, fmt.Errorf("check user sessions: %w", err)
		}
		if n > 0 {
			online[sessionUser[sessionID]] = true
		}
	}
	return online, nil
}

func uniqueUserIDs(userIDs []string) []string {
	seen := make(map[string]struct{}, len(userIDs))
	ids := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func authSessionKey(sessionID string) string   { return authSessionKeyPrefix + sessionID }
func authRefreshKey(digest string) string      { return authRefreshKeyPrefix + digest }
func authUserSessionsKey(userID string) string { return authUserSessionsKeyPrefix + userID }

// getRecord 读取并解码一份会话 JSON。
func (s *RedisAuthSessionStore) getRecord(ctx context.Context, sessionID string) (authSessionRecord, string, error) {
	if sessionID == "" {
		return authSessionRecord{}, "", ErrNotFound
	}
	recordJSON, err := s.client.Get(ctx, authSessionKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		return authSessionRecord{}, "", ErrNotFound
	}
	if err != nil {
		return authSessionRecord{}, "", fmt.Errorf("load authentication session: %w", err)
	}
	record, err := decodeAuthSessionRecord(recordJSON)
	if err != nil {
		return authSessionRecord{}, "", err
	}
	return record, recordJSON, nil
}

// encodeAuthSessionRecord 校验并序列化会话。
func encodeAuthSessionRecord(record authSessionRecord) (string, error) {
	if err := validateAuthSessionRecord(record); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode authentication session: %w", err)
	}
	return string(encoded), nil
}

// decodeAuthSessionRecord 反序列化并校验会话。
func decodeAuthSessionRecord(encoded string) (authSessionRecord, error) {
	var record authSessionRecord
	if err := json.Unmarshal([]byte(encoded), &record); err != nil {
		return authSessionRecord{}, fmt.Errorf("decode authentication session: %w", err)
	}
	if err := validateAuthSessionRecord(record); err != nil {
		return authSessionRecord{}, err
	}
	return record, nil
}

// validateAuthSessionRecord 检查会话必填字段。
func validateAuthSessionRecord(record authSessionRecord) error {
	if record.UserID == "" || record.AccessJTI == "" || record.RefreshDigest == "" || !record.Role.Valid() {
		return fmt.Errorf("invalid authentication session data")
	}
	return nil
}
