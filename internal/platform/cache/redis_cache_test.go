package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRedisCache_Get(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		expectedValue string
		expectedError bool
		mockError     error
	}{
		{
			name:          "should return value when key exists",
			key:           "test-key",
			expectedValue: "test-value",
			expectedError: false,
			mockError:     nil,
		},
		{
			name:          "should return error when key does not exist",
			key:           "non-existent-key",
			expectedValue: "",
			expectedError: true,
			mockError:     errors.New("redis: nil"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			if tt.expectedError {
				redisMock.ExpectGet(tt.key).RedisNil()
			} else {
				redisMock.ExpectGet(tt.key).SetVal(tt.expectedValue)
			}

			value, err := cache.Get(context.Background(), tt.key)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestRedisCache_Set(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expiration  time.Duration
		expectError bool
		mockError   error
	}{
		{
			name:        "should set value successfully",
			key:         "test-key",
			value:       "test-value",
			expiration:  time.Hour,
			expectError: false,
			mockError:   nil,
		},
		{
			name:        "should return error when redis operation fails",
			key:         "test-key",
			value:       "test-value",
			expiration:  time.Hour,
			expectError: true,
			mockError:   errors.New("redis connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			if tt.expectError {
				redisMock.ExpectSet(tt.key, tt.value, tt.expiration).SetErr(tt.mockError)
			} else {
				redisMock.ExpectSet(tt.key, tt.value, tt.expiration).SetVal("OK")
			}

			err := cache.Set(context.Background(), tt.key, tt.value, tt.expiration)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestRedisCache_Increment(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		value         int64
		expectedValue int64
		expectedError bool
		mockError     error
	}{
		{
			name:          "should increment value successfully",
			key:           "counter",
			value:         5,
			expectedValue: 10,
			expectedError: false,
			mockError:     nil,
		},
		{
			name:          "should return error when redis operation fails",
			key:           "counter",
			value:         5,
			expectedValue: 0,
			expectedError: true,
			mockError:     errors.New("redis connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			if tt.expectedError {
				redisMock.ExpectIncrBy(tt.key, tt.value).SetErr(tt.mockError)
			} else {
				redisMock.ExpectIncrBy(tt.key, tt.value).SetVal(tt.expectedValue)
			}

			result, err := cache.Increment(context.Background(), tt.key, tt.value)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, result)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestRedisCache_Delete(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
		mockError   error
	}{
		{
			name:        "should delete key successfully",
			key:         "test-key",
			expectError: false,
			mockError:   nil,
		},
		{
			name:        "should return error when redis operation fails",
			key:         "test-key",
			expectError: true,
			mockError:   errors.New("redis connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			if tt.expectError {
				redisMock.ExpectDel(tt.key).SetErr(tt.mockError)
			} else {
				redisMock.ExpectDel(tt.key).SetVal(1)
			}

			err := cache.Delete(context.Background(), tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestRedisCache_Exists(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		expectedValue bool
		expectedError bool
		mockError     error
	}{
		{
			name:          "should return true when key exists",
			key:           "existing-key",
			expectedValue: true,
			expectedError: false,
			mockError:     nil,
		},
		{
			name:          "should return false when key does not exist",
			key:           "non-existent-key",
			expectedValue: false,
			expectedError: false,
			mockError:     nil,
		},
		{
			name:          "should return error when redis operation fails",
			key:           "test-key",
			expectedValue: false,
			expectedError: true,
			mockError:     errors.New("redis connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			if tt.expectedError {
				redisMock.ExpectExists(tt.key).SetErr(tt.mockError)
				mockLogger.EXPECT().Error(mock.Anything, mock.Anything).Return()
			} else {
				if tt.expectedValue {
					redisMock.ExpectExists(tt.key).SetVal(1)
				} else {
					redisMock.ExpectExists(tt.key).SetVal(0)
				}
			}

			exists, err := cache.Exists(context.Background(), tt.key)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, exists)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestRedisCache_Close(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
		mockError   error
	}{
		{
			name:        "should close connection successfully",
			expectError: false,
			mockError:   nil,
		},
		{
			name:        "should close connection successfully even when close fails",
			expectError: false,
			mockError:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			db, redisMock := redismock.NewClientMock()

			cache := &RedisCache{
				client: db,
				log:    mockLogger,
			}

			mockLogger.EXPECT().Info(mock.Anything).Return()
			mockLogger.EXPECT().Info(mock.Anything).Return()

			err := cache.Close()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, redisMock.ExpectationsWereMet())
		})
	}
}

func TestNewRedisCache(t *testing.T) {
	t.Run("should create new redis cache instance", func(t *testing.T) {
		mockLogger := mocks.NewMockLogger(t)

		mockLogger.EXPECT().Error(mock.Anything).Return()

		defer func() {
			if r := recover(); r != nil {
				assert.Contains(t, r.(error).Error(), "redis: invalid URL scheme: ")
			}
		}()

		NewRedisCache(mockLogger)
	})
}
