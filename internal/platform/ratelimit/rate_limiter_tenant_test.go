package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiterTenant(t *testing.T) {
	c := newFakeCache()

	rateLimiter := NewRateLimiterTenant(c, fakeLogger{})

	assert.NotNil(t, rateLimiter)
}

func TestRateLimiterTenant_CheckLimit(t *testing.T) {
	tests := []struct {
		name            string
		tenantID        string
		endpoint        string
		currentCount    string
		getError        error
		ttl             time.Duration
		ttlError        error
		expectedAllowed bool
		expectedInfo    Info
		expectedError   bool
	}{
		{
			name:            "should allow when count is below limit",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    strconv.Itoa(constants.APIRateLimitTenantLimit / 2),
			getError:        nil,
			ttl:             30 * time.Second,
			ttlError:        nil,
			expectedAllowed: true,
			expectedInfo: Info{
				Limit:     constants.APIRateLimitTenantLimit,
				Remaining: constants.APIRateLimitTenantLimit - constants.APIRateLimitTenantLimit/2,
			},
			expectedError: false,
		},
		{
			name:            "should deny when count reaches limit",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    strconv.Itoa(constants.APIRateLimitTenantLimit),
			getError:        nil,
			ttl:             30 * time.Second,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo: Info{
				Limit:      constants.APIRateLimitTenantLimit,
				Remaining:  0,
				RetryAfter: 30,
			},
			expectedError: false,
		},
		{
			name:            "should allow when key does not exist",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    "",
			getError:        errors.New("redis: nil"),
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: true,
			expectedInfo: Info{
				Limit:      constants.APIRateLimitTenantLimit,
				Remaining:  constants.APIRateLimitTenantLimit,
				RetryAfter: 0,
			},
			expectedError: false,
		},
		{
			name:            "should return error when get fails with non-nil error",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    "",
			getError:        errors.New("redis connection error"),
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo:    Info{},
			expectedError:   true,
		},
		{
			name:            "should return error when parsing count fails",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    "invalid",
			getError:        nil,
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo:    Info{},
			expectedError:   true,
		},
		{
			name:            "should handle TTL error gracefully",
			tenantID:        "tenant-123",
			endpoint:        "/api/v1/comments",
			currentCount:    strconv.Itoa(constants.APIRateLimitTenantLimit / 2),
			getError:        nil,
			ttl:             constants.APIRateLimitWindow,
			ttlError:        errors.New("ttl error"),
			expectedAllowed: true,
			expectedInfo: Info{
				Limit:     constants.APIRateLimitTenantLimit,
				Remaining: constants.APIRateLimitTenantLimit - constants.APIRateLimitTenantLimit/2,
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeCache()

			rateLimiter := NewRateLimiterTenant(c, fakeLogger{})

			if tt.getError != nil && tt.getError.Error() == "redis: nil" {
				c.get = func() (string, error) { return "", tt.getError }
			} else if tt.getError != nil {
				c.get = func() (string, error) { return "", tt.getError }
			} else if tt.currentCount == "invalid" {
				c.get = func() (string, error) { return tt.currentCount, nil }
			} else {
				c.get = func() (string, error) { return tt.currentCount, nil }
				if tt.currentCount != "" {
					if tt.ttlError != nil {
						c.ttl = func() (time.Duration, error) { return 0, tt.ttlError }
					} else {
						c.ttl = func() (time.Duration, error) { return tt.ttl, nil }
					}
				}
			}

			allowed, info, err := rateLimiter.CheckLimit(context.Background(), tt.tenantID, tt.endpoint)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAllowed, allowed)
				assert.Equal(t, tt.expectedInfo.Limit, info.Limit)
				assert.Equal(t, tt.expectedInfo.Remaining, info.Remaining)
				if tt.expectedInfo.RetryAfter > 0 {
					assert.Equal(t, tt.expectedInfo.RetryAfter, info.RetryAfter)
				}
			}
		})
	}
}

func TestRateLimiterTenant_Increment(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		endpoint       string
		exists         bool
		existsError    error
		increment      int64
		incrementError error
		setError       error
		expectedError  bool
	}{
		{
			name:           "should increment successfully when key exists",
			tenantID:       "tenant-123",
			endpoint:       "/api/v1/comments",
			exists:         true,
			existsError:    nil,
			increment:      51,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should increment and set expiration when key does not exist",
			tenantID:       "tenant-123",
			endpoint:       "/api/v1/comments",
			exists:         false,
			existsError:    nil,
			increment:      1,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should return error when exists check fails",
			tenantID:       "tenant-123",
			endpoint:       "/api/v1/comments",
			exists:         false,
			existsError:    errors.New("redis error"),
			increment:      0,
			incrementError: nil,
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when increment fails",
			tenantID:       "tenant-123",
			endpoint:       "/api/v1/comments",
			exists:         true,
			existsError:    nil,
			increment:      0,
			incrementError: errors.New("redis error"),
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when set expiration fails",
			tenantID:       "tenant-123",
			endpoint:       "/api/v1/comments",
			exists:         false,
			existsError:    nil,
			increment:      1,
			incrementError: nil,
			setError:       errors.New("redis error"),
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeCache()

			rateLimiter := NewRateLimiterTenant(c, fakeLogger{})

			c.exists = func() (bool, error) { return tt.exists, tt.existsError }

			if tt.existsError != nil {
			} else {
				c.increment = func() (int64, error) { return tt.increment, tt.incrementError }

				if tt.incrementError != nil {
				} else {
					if !tt.exists {
						c.set = tt.setError

						if tt.setError != nil {
						}
					}
				}
			}

			err := rateLimiter.Increment(context.Background(), tt.tenantID, tt.endpoint)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
