package data

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/buding00/springhere-gin-server/internal/entity"
	"github.com/buding00/springhere-gin-server/pkg/constant"
	"github.com/redis/go-redis/v9"
)

func TestOnlineMapActiveSession(t *testing.T) {
	store, _ := newTestSessionStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, "sid-1", testSession("user-1"), "digest-1", time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	online, err := store.OnlineMap(ctx, []string{"user-1", "user-2", "user-1", ""})
	if err != nil {
		t.Fatalf("online map: %v", err)
	}
	if !online["user-1"] {
		t.Fatal("expected user-1 to be online")
	}
	if online["user-2"] {
		t.Fatal("expected user-2 to be offline")
	}
}

func TestOnlineMapAfterDelete(t *testing.T) {
	store, _ := newTestSessionStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, "sid-1", testSession("user-1"), "digest-1", time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.DeleteUserSessions(ctx, "user-1"); err != nil {
		t.Fatalf("delete sessions: %v", err)
	}
	online, err := store.OnlineMap(ctx, []string{"user-1"})
	if err != nil {
		t.Fatalf("online map: %v", err)
	}
	if online["user-1"] {
		t.Fatal("expected user-1 to be offline after kick")
	}
}

func TestOnlineMapIgnoresStaleIndex(t *testing.T) {
	store, mr := newTestSessionStore(t)
	ctx := context.Background()
	if _, err := mr.ZAdd(authUserSessionsKey("user-1"), float64(time.Now().Add(time.Hour).UnixMilli()), "missing-sid"); err != nil {
		t.Fatalf("seed stale index: %v", err)
	}
	online, err := store.OnlineMap(ctx, []string{"user-1"})
	if err != nil {
		t.Fatalf("online map: %v", err)
	}
	if online["user-1"] {
		t.Fatal("expected stale session index to count as offline")
	}
}

func TestOnlineMapExpiredSession(t *testing.T) {
	store, mr := newTestSessionStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, "sid-1", testSession("user-1"), "digest-1", time.Minute); err != nil {
		t.Fatalf("create session: %v", err)
	}
	mr.FastForward(2 * time.Minute)
	online, err := store.OnlineMap(ctx, []string{"user-1"})
	if err != nil {
		t.Fatalf("online map: %v", err)
	}
	if online["user-1"] {
		t.Fatal("expected expired session to count as offline")
	}
}

func newTestSessionStore(t *testing.T) (*RedisAuthSessionStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisAuthSessionStore(client), mr
}

func testSession(userID string) entity.AuthSession {
	return entity.AuthSession{
		UserID:    userID,
		Email:     "user@example.com",
		Remark:    "user",
		Role:      constant.RoleUser,
		Active:    true,
		AccessJTI: "jti-" + userID,
	}
}
