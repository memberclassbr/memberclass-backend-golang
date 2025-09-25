package cache

import (
	"context"
	"os"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	log    ports.Logger
}

func NewRedisCache(log ports.Logger) ports.Cache {
	client := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("UPSTASH_REDIS_REST_URL"),
		Password: os.Getenv("UPSTASH_REDIS_REST_TOKEN"),
	})

	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		log.Error("Failed to connect to Redis: " + err.Error())
		panic(err)
	}

	log.Info("Redis connection established successfully")

	return &RedisCache{
		client: client,
		log:    log,
	}
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
