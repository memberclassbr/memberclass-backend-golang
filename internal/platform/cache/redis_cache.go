package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	log    logger.Logger
}

// NewRedisCache connects to Redis. It returns an error rather than panicking:
// an unreachable cache is a startup failure the composition root reports, not
// a stack trace on stderr.
func NewRedisCache(cfg *config.Config, log logger.Logger) (Cache, error) {
	redisURL := cfg.Redis.URL

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}

	// rediss:// means TLS; ParseURL only sets it for some URL shapes, so it is
	// forced here when the scheme asks for it.
	if opts.TLSConfig == nil && strings.HasPrefix(redisURL, "rediss://") {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	client := redis.NewClient(opts)

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}

	log.Info("Redis connection established successfully")

	return &RedisCache{client: client, log: log}, nil
}

func (u *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return u.client.Get(ctx, key).Result()
}

func (u *RedisCache) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	return u.client.Set(ctx, key, value, expiration).Err()
}

func (u *RedisCache) Increment(ctx context.Context, key string, value int64) (int64, error) {
	return u.client.IncrBy(ctx, key, value).Result()
}

func (u *RedisCache) Delete(ctx context.Context, key string) error {
	return u.client.Del(ctx, key).Err()
}

func (u *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := u.client.Exists(ctx, key).Result()
	if err != nil {
		u.log.Error("Error checking if key exists", err.Error())
		return false, err
	}
	return result > 0, nil
}

func (u *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := u.client.TTL(ctx, key).Result()
	if err != nil {
		u.log.Error("Error getting TTL for key " + key + ": " + err.Error())
		return 0, err
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

func (u *RedisCache) Close() error {
	if u.client != nil {
		u.log.Info("Closing Redis connection...")
		err := u.client.Close()
		if err != nil {
			u.log.Error("Error closing Redis connection: " + err.Error())
			return err
		}
		u.log.Info("Redis connection closed successfully")
	}
	return nil
}
