package rate_limiter

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/ports/rate_limit"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewRateLimiterIP(t *testing.T) {
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	rateLimiter := NewRateLimiterIP(mockCache, mockLogger)

	assert.NotNil(t, rateLimiter)
}

func TestRateLimiterIP_CheckLimit(t *testing.T) {
	tests := []struct {
		name            string
		ip              string
		currentCount    string
		getError        error
		ttl             time.Duration
		ttlError        error
		expectedAllowed bool
		expectedInfo    rate_limit.RateLimitInfo
		expectedError   bool
	}{
		{
			name:            "should allow when count is below limit",
			ip:              "192.168.1.1",
			currentCount:    "25",
			getError:        nil,
			ttl:             30 * time.Second,
			ttlError:        nil,
			expectedAllowed: true,
			expectedInfo: rate_limit.RateLimitInfo{
				Limit:     constants.APIRateLimitIPLimit,
				Remaining: 25,
			},
			expectedError: false,
		},
		{
			name:            "should deny when count reaches limit",
			ip:              "192.168.1.1",
			currentCount:    "60",
			getError:        nil,
			ttl:             30 * time.Second,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo: rate_limit.RateLimitInfo{
				Limit:      constants.APIRateLimitIPLimit,
				Remaining:  0,
				RetryAfter: 30,
			},
			expectedError: false,
		},
		{
			name:            "should allow when key does not exist",
			ip:              "192.168.1.1",
			currentCount:    "",
			getError:        errors.New("redis: nil"),
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: true,
			expectedInfo: rate_limit.RateLimitInfo{
				Limit:      constants.APIRateLimitIPLimit,
				Remaining:  constants.APIRateLimitIPLimit,
				RetryAfter: 0,
			},
			expectedError: false,
		},
		{
			name:            "should return error when get fails with non-nil error",
			ip:              "192.168.1.1",
			currentCount:    "",
			getError:        errors.New("redis connection error"),
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo:    rate_limit.RateLimitInfo{},
			expectedError:   true,
		},
		{
			name:            "should return error when parsing count fails",
			ip:              "192.168.1.1",
			currentCount:    "invalid",
			getError:        nil,
			ttl:             0,
			ttlError:        nil,
			expectedAllowed: false,
			expectedInfo:    rate_limit.RateLimitInfo{},
			expectedError:   true,
		},
		{
			name:            "should handle TTL error gracefully",
			ip:              "192.168.1.1",
			currentCount:    "25",
			getError:        nil,
			ttl:             constants.APIRateLimitWindow,
			ttlError:        errors.New("ttl error"),
			expectedAllowed: true,
			expectedInfo: rate_limit.RateLimitInfo{
				Limit:     constants.APIRateLimitIPLimit,
				Remaining: 25,
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			rateLimiter := NewRateLimiterIP(mockCache, mockLogger)

			key := constants.APIRateLimitIPKeyPrefix + tt.ip

			if tt.getError != nil && tt.getError.Error() == "redis: nil" {
				mockCache.EXPECT().Get(mock.Anything, key).Return("", tt.getError)
			} else if tt.getError != nil {
				mockCache.EXPECT().Get(mock.Anything, key).Return("", tt.getError)
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
			} else if tt.currentCount == "invalid" {
				mockCache.EXPECT().Get(mock.Anything, key).Return(tt.currentCount, nil)
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
			} else {
				mockCache.EXPECT().Get(mock.Anything, key).Return(tt.currentCount, nil)
				if tt.currentCount != "" {
					if tt.ttlError != nil {
						mockCache.EXPECT().TTL(mock.Anything, key).Return(time.Duration(0), tt.ttlError)
						mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
					} else {
						mockCache.EXPECT().TTL(mock.Anything, key).Return(tt.ttl, nil)
					}
				}
			}

			allowed, info, err := rateLimiter.CheckLimit(context.Background(), tt.ip)

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

func TestRateLimiterIP_Increment(t *testing.T) {
	tests := []struct {
		name           string
		ip             string
		exists         bool
		existsError    error
		increment      int64
		incrementError error
		setError       error
		expectedError  bool
	}{
		{
			name:           "should increment successfully when key exists",
			ip:             "192.168.1.1",
			exists:         true,
			existsError:    nil,
			increment:      26,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should increment and set expiration when key does not exist",
			ip:             "192.168.1.1",
			exists:         false,
			existsError:    nil,
			increment:      1,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should return error when exists check fails",
			ip:             "192.168.1.1",
			exists:         false,
			existsError:    errors.New("redis error"),
			increment:      0,
			incrementError: nil,
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when increment fails",
			ip:             "192.168.1.1",
			exists:         true,
			existsError:    nil,
			increment:      0,
			incrementError: errors.New("redis error"),
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when set expiration fails",
			ip:             "192.168.1.1",
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
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			rateLimiter := NewRateLimiterIP(mockCache, mockLogger)

			key := constants.APIRateLimitIPKeyPrefix + tt.ip

			mockCache.EXPECT().Exists(mock.Anything, key).Return(tt.exists, tt.existsError)

			if tt.existsError != nil {
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
			} else {
				mockCache.EXPECT().Increment(mock.Anything, key, int64(1)).Return(tt.increment, tt.incrementError)

				if tt.incrementError != nil {
					mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
				} else {
					if !tt.exists {
						mockCache.EXPECT().Set(mock.Anything, key, strconv.FormatInt(tt.increment, 10), constants.APIRateLimitWindow).Return(tt.setError)

						if tt.setError != nil {
							mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
						}
					}
				}
			}

			err := rateLimiter.Increment(context.Background(), tt.ip)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
