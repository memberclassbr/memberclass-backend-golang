package ratelimit

import (
	"context"
	"errors"
	"time"
)

// fakeCache is a programmable stand-in for the Redis cache. Each field, when
// set, decides what that method returns; unset methods behave as a cache miss
// or a no-op. The limiters only ever touch one key per call, so the stubs take
// no key argument.
type fakeCache struct {
	get       func() (string, error)
	set       error
	increment func() (int64, error)
	exists    func() (bool, error)
	ttl       func() (time.Duration, error)

	// setCalls records the values written, so a test can assert the counter
	// the limiter persisted.
	setCalls []string
}

func newFakeCache() *fakeCache { return &fakeCache{} }

func (c *fakeCache) Get(context.Context, string) (string, error) {
	if c.get == nil {
		return "", errors.New("cache miss")
	}
	return c.get()
}

func (c *fakeCache) Set(_ context.Context, _ string, value string, _ time.Duration) error {
	c.setCalls = append(c.setCalls, value)
	return c.set
}

func (c *fakeCache) Increment(context.Context, string, int64) (int64, error) {
	if c.increment == nil {
		return 0, nil
	}
	return c.increment()
}

func (c *fakeCache) Exists(context.Context, string) (bool, error) {
	if c.exists == nil {
		return false, nil
	}
	return c.exists()
}

func (c *fakeCache) TTL(context.Context, string) (time.Duration, error) {
	if c.ttl == nil {
		return 0, nil
	}
	return c.ttl()
}

func (c *fakeCache) Delete(context.Context, string) error { return nil }
func (c *fakeCache) Close() error                         { return nil }

// fakeLogger discards output: these tests assert on limiter decisions, not on
// what was logged.
type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}
