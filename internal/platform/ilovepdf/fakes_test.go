package ilovepdf

import (
	"context"
	"errors"
	"sync"
	"time"
)

// fakeCache behaves like a real cache rather than asserting on calls: the key
// blacklist is a set, so "not blacklisted → blacklist → blacklisted" falls out
// of the implementation instead of being scripted per test.
type fakeCache struct {
	mu    sync.Mutex
	store map[string]string
	// setErr, when set, makes every write fail — used to check that a failed
	// blacklist write surfaces.
	setErr error
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]string{}} }

// blacklist seeds a key as already exhausted.
func (c *fakeCache) blacklist(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store["ilovepdf_blacklist:"+key] = "exhausted"
}

func (c *fakeCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.store[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return v, nil
}

func (c *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
	return nil
}

func (c *fakeCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.store[key]
	return ok, nil
}

func (c *fakeCache) Increment(context.Context, string, int64) (int64, error) { return 0, nil }
func (c *fakeCache) Delete(context.Context, string) error                    { return nil }
func (c *fakeCache) TTL(context.Context, string) (time.Duration, error)      { return 0, nil }
func (c *fakeCache) Close() error                                            { return nil }

// fakeLogger discards output: these tests assert on API behaviour, not logs.
type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}
