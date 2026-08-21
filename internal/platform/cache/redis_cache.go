package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

type RedisCache struct {
	client *redis.Client
	log    logger.Logger
}

// connectAttempts and connectBackoff cover the window where a platform's
// private network is not resolvable yet. On Railway the internal DNS zone
// (*.railway.internal, AAAA-only) comes up a beat after the container does, so
// the first Ping can fail with "no such host" on a perfectly valid URL.
const (
	connectAttempts = 6
	connectBackoff  = 2 * time.Second
)

// NewRedisCache connects to Redis. It returns an error rather than panicking:
// an unreachable cache is a startup failure the composition root reports, not
// a stack trace on stderr.
//
// The return type is HashCache, the wider of the two interfaces, so the
// composition root can hand the same value to a caller that needs the hash
// operations and to one that only takes Cache.
func NewRedisCache(cfg *config.Config, log logger.Logger) (HashCache, error) {
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

	// Every rate-limiter decision goes through this client, so a Redis that
	// slows down shows up as latency on endpoints that have nothing to do with
	// caching. Instrumenting it is what makes that attributable.
	if err := redisotel.InstrumentTracing(client); err != nil {
		log.Warn("Redis tracing unavailable: " + err.Error())
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		log.Warn("Redis metrics unavailable: " + err.Error())
	}

	var pingErr error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		if _, pingErr = client.Ping(context.Background()).Result(); pingErr == nil {
			break
		}
		if attempt < connectAttempts {
			log.Warn(fmt.Sprintf(
				"Redis not reachable at %s (attempt %d/%d): %s — retrying in %s",
				opts.Addr, attempt, connectAttempts, pingErr, connectBackoff,
			))
			time.Sleep(connectBackoff)
		}
	}
	if pingErr != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Redis at %s after %d attempts: %w", opts.Addr, connectAttempts, pingErr)
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

// HIncrByPipeline applies every increment and slides each touched hash's TTL
// forward, in one round trip.
//
// The TTL is rewritten on every call rather than only on the first, which
// costs one pipelined command and saves having to know whether the hash
// already existed — a question that cannot be answered without another round
// trip, which is the thing being avoided.
func (u *RedisCache) HIncrByPipeline(ctx context.Context, ttl time.Duration, incs ...HashIncr) error {
	if len(incs) == 0 {
		return nil
	}

	pipe := u.client.Pipeline()
	touched := make(map[string]struct{}, len(incs))
	for _, inc := range incs {
		pipe.HIncrBy(ctx, inc.Key, inc.Field, inc.By)
		touched[inc.Key] = struct{}{}
	}
	for key := range touched {
		pipe.Expire(ctx, key, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (u *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return u.client.HGetAll(ctx, key).Result()
}

func (u *RedisCache) SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return u.client.SetNX(ctx, key, value, expiration).Result()
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
