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

// HashIncr is one field increment inside a hash, as applied by
// HIncrByPipeline.
type HashIncr struct {
	Key   string
	Field string
	By    int64
}

// HashCache is Cache plus the hash and lock operations the API-key usage
// counters need.
//
// It is a second interface rather than four more methods on Cache because
// every fake in the test suite implements Cache, and widening that would make
// adding a Redis command a nine-file change. NewRedisCache returns HashCache,
// which is assignable to Cache, so a caller that needs only the plain
// key-value operations keeps taking Cache and never sees this.
type HashCache interface {
	Cache

	// HIncrByPipeline applies every increment and refreshes each touched
	// hash's TTL in a single round trip. The API-key recorder runs inside the
	// request, so the number of round trips is the whole point of the method.
	HIncrByPipeline(ctx context.Context, ttl time.Duration, incs ...HashIncr) error

	// HGetAll reads a whole hash. Callers must keep hashes small enough that
	// this is not a latency event — the usage hashes are bounded by
	// keys x endpoints, in the thousands.
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// SetNX writes key only if it does not exist, reporting whether it won.
	// It is how a scheduled job elects one replica to run.
	SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
}
