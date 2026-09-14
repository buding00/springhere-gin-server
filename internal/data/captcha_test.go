package data

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCaptchaStoreConsumesOnce(t *testing.T) {
	store, _ := newTestCaptchaStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, "id-1", "AB12", time.Minute); err != nil {
		t.Fatalf("put captcha: %v", err)
	}
	ok, err := store.Consume(ctx, "id-1", "ab12")
	if err != nil {
		t.Fatalf("consume captcha: %v", err)
	}
	if !ok {
		t.Fatal("expected matching captcha to succeed")
	}
	ok, err = store.Consume(ctx, "id-1", "ab12")
	if err != nil {
		t.Fatalf("consume captcha again: %v", err)
	}
	if ok {
		t.Fatal("expected captcha to be one-time")
	}
}

func TestCaptchaStoreDeletesOnMismatch(t *testing.T) {
	store, _ := newTestCaptchaStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, "id-1", "AB12", time.Minute); err != nil {
		t.Fatalf("put captcha: %v", err)
	}
	ok, err := store.Consume(ctx, "id-1", "ZZZZ")
	if err != nil {
		t.Fatalf("consume captcha: %v", err)
	}
	if ok {
		t.Fatal("expected mismatch to fail")
	}
	ok, err = store.Consume(ctx, "id-1", "AB12")
	if err != nil {
		t.Fatalf("consume captcha after mismatch: %v", err)
	}
	if ok {
		t.Fatal("expected mismatched captcha to be deleted")
	}
}

func TestCaptchaStoreUnknownAndExpired(t *testing.T) {
	store, mr := newTestCaptchaStore(t)
	ctx := context.Background()
	ok, err := store.Consume(ctx, "missing", "AB12")
	if err != nil {
		t.Fatalf("consume missing captcha: %v", err)
	}
	if ok {
		t.Fatal("expected unknown captcha to fail")
	}
	if err := store.Put(ctx, "id-1", "AB12", time.Minute); err != nil {
		t.Fatalf("put captcha: %v", err)
	}
	mr.FastForward(2 * time.Minute)
	ok, err = store.Consume(ctx, "id-1", "AB12")
	if err != nil {
		t.Fatalf("consume expired captcha: %v", err)
	}
	if ok {
		t.Fatal("expected expired captcha to fail")
	}
}

func TestCaptchaStoreIssueLimit(t *testing.T) {
	store, _ := newTestCaptchaStore(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		allowed, err := store.AllowIssue(ctx, "127.0.0.1", 2, time.Minute)
		if err != nil {
			t.Fatalf("allow issue: %v", err)
		}
		if !allowed {
			t.Fatalf("expected issue %d to be allowed", i+1)
		}
	}
	allowed, err := store.AllowIssue(ctx, "127.0.0.1", 2, time.Minute)
	if err != nil {
		t.Fatalf("allow issue over limit: %v", err)
	}
	if allowed {
		t.Fatal("expected issue limit to reject extra requests")
	}
	allowed, err = store.AllowIssue(ctx, "10.0.0.2", 2, time.Minute)
	if err != nil {
		t.Fatalf("allow issue for other ip: %v", err)
	}
	if !allowed {
		t.Fatal("expected a different IP to have its own limit")
	}
}

func newTestCaptchaStore(t *testing.T) (*RedisCaptchaStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisCaptchaStore(client), mr
}
