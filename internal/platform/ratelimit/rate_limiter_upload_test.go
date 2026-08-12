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

func TestRateLimiterUpload_CheckUploadLimit(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		fileSize       int64
		currentSize    int64
		getError       error
		expectedResult UploadResult
		expectedError  bool
	}{
		{
			name:        "should allow upload when within limit",
			key:         "user123",
			fileSize:    1024 * 1024,
			currentSize: 5 * 1024 * 1024 * 1024,
			getError:    nil,
			expectedResult: UploadResult{
				Allowed:       true,
				CurrentSize:   5 * 1024 * 1024 * 1024,
				MaxSize:       constants.MaxUploadSizePerDay,
				RemainingSize: 5 * 1024 * 1024 * 1024,
				ResetTime:     time.Now().Add(constants.UploadLimitExpiration).Unix(),
			},
			expectedError: false,
		},
		{
			name:        "should deny upload when exceeding limit",
			key:         "user123",
			fileSize:    6 * 1024 * 1024 * 1024,
			currentSize: 5 * 1024 * 1024 * 1024,
			getError:    nil,
			expectedResult: UploadResult{
				Allowed:       false,
				CurrentSize:   5 * 1024 * 1024 * 1024,
				MaxSize:       constants.MaxUploadSizePerDay,
				RemainingSize: 5 * 1024 * 1024 * 1024,
				ResetTime:     time.Now().Add(constants.UploadLimitExpiration).Unix(),
			},
			expectedError: false,
		},
		{
			name:        "should handle zero current size",
			key:         "user123",
			fileSize:    1024 * 1024,
			currentSize: 0,
			getError:    nil,
			expectedResult: UploadResult{
				Allowed:       true,
				CurrentSize:   0,
				MaxSize:       constants.MaxUploadSizePerDay,
				RemainingSize: constants.MaxUploadSizePerDay,
				ResetTime:     time.Now().Add(constants.UploadLimitExpiration).Unix(),
			},
			expectedError: false,
		},
		{
			name:           "should return error when get current size fails",
			key:            "user123",
			fileSize:       1024 * 1024,
			currentSize:    0,
			getError:       errors.New("redis error"),
			expectedResult: UploadResult{},
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeCache()

			rateLimiter := NewRateLimiterUpload(c, fakeLogger{})

			if tt.getError != nil {
				c.get = func() (string, error) { return "", tt.getError }
			} else {
				c.get = func() (string, error) { return strconv.FormatInt(tt.currentSize, 10), nil }
			}

			result, err := rateLimiter.CheckUploadLimit(context.Background(), tt.key, tt.fileSize)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, UploadResult{}, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Allowed, result.Allowed)
				assert.Equal(t, tt.expectedResult.CurrentSize, result.CurrentSize)
				assert.Equal(t, tt.expectedResult.MaxSize, result.MaxSize)
				assert.Equal(t, tt.expectedResult.RemainingSize, result.RemainingSize)
				assert.WithinDuration(t, time.Unix(tt.expectedResult.ResetTime, 0), time.Unix(result.ResetTime, 0), time.Second)
			}
		})
	}
}

func TestRateLimiterUpload_IncrementUploadSize(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		fileSize       int64
		exists         bool
		existsError    error
		increment      int64
		incrementError error
		setError       error
		expectedError  bool
	}{
		{
			name:           "should increment upload size successfully when key exists",
			key:            "user123",
			fileSize:       1024 * 1024,
			exists:         true,
			existsError:    nil,
			increment:      1024 * 1024,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should increment upload size and set expiration when key does not exist",
			key:            "user123",
			fileSize:       1024 * 1024,
			exists:         false,
			existsError:    nil,
			increment:      1024 * 1024,
			incrementError: nil,
			setError:       nil,
			expectedError:  false,
		},
		{
			name:           "should return error when exists check fails",
			key:            "user123",
			fileSize:       1024 * 1024,
			exists:         false,
			existsError:    errors.New("redis error"),
			increment:      0,
			incrementError: nil,
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when increment fails",
			key:            "user123",
			fileSize:       1024 * 1024,
			exists:         true,
			existsError:    nil,
			increment:      0,
			incrementError: errors.New("redis error"),
			setError:       nil,
			expectedError:  true,
		},
		{
			name:           "should return error when set expiration fails",
			key:            "user123",
			fileSize:       1024 * 1024,
			exists:         false,
			existsError:    nil,
			increment:      1024 * 1024,
			incrementError: nil,
			setError:       errors.New("redis error"),
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeCache()

			rateLimiter := NewRateLimiterUpload(c, fakeLogger{})

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

					if tt.setError == nil {
					}
				}
			}

			err := rateLimiter.IncrementUploadSize(context.Background(), tt.key, tt.fileSize)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRateLimiterUpload_GetCurrentUploadSize(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		cacheValue    string
		cacheError    error
		expectedSize  int64
		expectedError bool
	}{
		{
			name:          "should return current size when key exists",
			key:           "user123",
			cacheValue:    "1048576",
			cacheError:    nil,
			expectedSize:  1048576,
			expectedError: false,
		},
		{
			name:          "should return zero when key does not exist",
			key:           "user123",
			cacheValue:    "",
			cacheError:    errors.New("redis: nil"),
			expectedSize:  0,
			expectedError: false,
		},
		{
			name:          "should return error when cache operation fails",
			key:           "user123",
			cacheValue:    "",
			cacheError:    errors.New("redis connection error"),
			expectedSize:  0,
			expectedError: true,
		},
		{
			name:          "should return error when parsing fails",
			key:           "user123",
			cacheValue:    "invalid",
			cacheError:    nil,
			expectedSize:  0,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeCache()

			rateLimiter := NewRateLimiterUpload(c, fakeLogger{})

			c.get = func() (string, error) { return tt.cacheValue, tt.cacheError }

			if tt.cacheError == nil && tt.expectedError {
			}

			size, err := rateLimiter.GetCurrentUploadSize(context.Background(), tt.key)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, int64(0), size)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSize, size)
			}
		})
	}
}
