// Package cache provides the Redis-backed key-value store used for response
// caching and as the counter store behind the rate limiters.
package cache

import (
	"context"
	"time"
)

// Cache is the contract satisfied by the Redis implementation in this package.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, expiration time.Duration) error
	Increment(ctx context.Context, key string, value int64) (int64, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Close() error
}
