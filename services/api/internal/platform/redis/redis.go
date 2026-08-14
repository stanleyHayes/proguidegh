// Package redis sets up the Redis client used for ephemeral state only
// (presence, locks, idempotency cache, rate limits, offer TTLs).
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Connect parses a redis:// URL, builds a client, and verifies it with a
// ping. The caller owns the client and must Close it.
func Connect(ctx context.Context, redisURL string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	client := goredis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}
	return client, nil
}
