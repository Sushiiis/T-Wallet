package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
}

func New(rdb *redis.Client, limit int64, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, limit: limit, window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	windowID := time.Now().Unix() / int64(l.window.Seconds())
	redisKey := fmt.Sprintf("ratelimit:%s:%d", key, windowID)

	count, err := l.rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("incr rate limit key: %w", err)
	}
	if count == 1 {
		if err := l.rdb.Expire(ctx, redisKey, l.window).Err(); err != nil {
			return false, fmt.Errorf("expire rate limit key: %w", err)
		}
	}
	return count <= l.limit, nil
}