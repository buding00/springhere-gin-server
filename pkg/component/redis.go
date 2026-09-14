package component

import (
	"context"
	"fmt"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/redis/go-redis/v9"
)

// NewRedis creates the Redis client used by authentication session storage.
func NewRedis(cfg config.RedisConfig) (*redis.Client, func()) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		panic(fmt.Errorf("initialize Redis: ping server: %w", err))
	}
	return client, func() { _ = client.Close() }
}

func PingRedis(ctx context.Context, client redis.Cmdable) error {
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}
