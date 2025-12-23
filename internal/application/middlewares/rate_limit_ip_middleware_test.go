package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewRateLimitIPMiddleware(t *testing.T) {
	mockRateLimiter := mocks.NewMockRateLimiterIP(t)
	mockLogger := mocks.NewMockLogger(t)

	middleware := NewRateLimitIPMiddleware(mockRateLimiter, mockLogger)

	assert.NotNil(t, middleware)
	assert.Equal(t, mockRateLimiter, middleware.rateLimiter)
	assert.Equal(t, mockLogger, middleware.logger)
}

func TestRateLimitIPMiddleware_LimitByIP(t *testing.T) {
	tests := []struct {
		name              string
		ip                string
		allowed           bool
		rateLimitInfo     ports.RateLimitInfo
		checkLimitError   error
		incrementError    error
		expectedStatusCode int
		shouldCallNext    bool
		expectHeaders     bool
		expectErrorBody   bool
		headers           map[string]string
	}{
		{
			name:   "should allow request when within limit",
			ip:     "192.168.1.1",
			allowed: true,
			rateLimitInfo: ports.RateLimitInfo{
				Limit:     60,
				Remaining: 30,
				Reset:     time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:   nil,
			incrementError:    nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     true,
			expectErrorBody:   false,
			headers:           map[string]string{},
		},
		{
			name:   "should deny request when limit exceeded",
			ip:     "192.168.1.1",
			allowed: false,
			rateLimitInfo: ports.RateLimitInfo{
				Limit:     60,
				Remaining: 0,
				Reset:     time.Now().Add(30 * time.Second),
				RetryAfter: 30,
			},
			checkLimitError:   nil,
			incrementError:    nil,
			expectedStatusCode: http.StatusTooManyRequests,
			shouldCallNext:    false,
			expectHeaders:     true,
			expectErrorBody:   true,
			headers:           map[string]string{},
		},
		{
			name:              "should pass through when IP is empty",
			ip:                "",
			allowed:           false,
			rateLimitInfo:     ports.RateLimitInfo{},
			checkLimitError:   nil,
			incrementError:    nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     false,
			expectErrorBody:   false,
			headers:           map[string]string{},
		},
		{
			name:   "should pass through when check limit fails",
			ip:     "192.168.1.1",
			allowed: false,
			rateLimitInfo: ports.RateLimitInfo{},
			checkLimitError:   assert.AnError,
			incrementError:    nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     false,
			expectErrorBody:   false,
			headers:           map[string]string{},
		},
		{
			name:   "should continue when increment fails",
			ip:     "192.168.1.1",
			allowed: true,
			rateLimitInfo: ports.RateLimitInfo{
				Limit:     60,
				Remaining: 30,
				Reset:     time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:   nil,
			incrementError:    assert.AnError,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     true,
			expectErrorBody:   false,
			headers:           map[string]string{},
		},
		{
			name:   "should use X-Forwarded-For header when present",
			ip:     "10.0.0.1",
			allowed: true,
			rateLimitInfo: ports.RateLimitInfo{
				Limit:     60,
				Remaining: 30,
				Reset:     time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:   nil,
			incrementError:    nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     true,
			expectErrorBody:   false,
			headers:           map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.1"},
		},
		{
			name:   "should use X-Real-IP header when present",
			ip:     "172.16.0.1",
			allowed: true,
			rateLimitInfo: ports.RateLimitInfo{
				Limit:     60,
				Remaining: 30,
				Reset:     time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:   nil,
			incrementError:    nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:    true,
			expectHeaders:     true,
			expectErrorBody:   false,
			headers:           map[string]string{"X-Real-IP": "172.16.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRateLimiter := mocks.NewMockRateLimiterIP(t)
			mockLogger := mocks.NewMockLogger(t)

			middleware := NewRateLimitIPMiddleware(mockRateLimiter, mockLogger)

			nextHandlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextHandlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			if tt.ip != "" && len(tt.headers) == 0 {
				req.RemoteAddr = tt.ip + ":12345"
			} else if tt.ip == "" {
				req.RemoteAddr = ""
			}

			if tt.ip != "" && tt.checkLimitError == nil {
				mockRateLimiter.EXPECT().CheckLimit(mock.Anything, tt.ip).Return(tt.allowed, tt.rateLimitInfo, nil)
			} else if tt.ip != "" && tt.checkLimitError != nil {
				mockRateLimiter.EXPECT().CheckLimit(mock.Anything, tt.ip).Return(false, ports.RateLimitInfo{}, tt.checkLimitError)
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
			}

			if tt.allowed && tt.checkLimitError == nil && tt.ip != "" {
				if tt.incrementError != nil {
					mockRateLimiter.EXPECT().Increment(mock.Anything, tt.ip).Return(tt.incrementError)
					mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
				} else {
					mockRateLimiter.EXPECT().Increment(mock.Anything, tt.ip).Return(nil)
				}
			}

			rec := httptest.NewRecorder()

			handler := middleware.LimitByIP(nextHandler)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)
			assert.Equal(t, tt.shouldCallNext, nextHandlerCalled)

			if tt.expectHeaders {
				assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Limit"))
				assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Remaining"))
				assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
			}

			if tt.expectErrorBody {
				var response map[string]interface{}
				err := json.NewDecoder(rec.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.NotEmpty(t, response["error"])
				assert.Equal(t, "RATE_LIMIT_EXCEEDED", response["errorCode"])
				assert.NotNil(t, response["retryAfter"])
				assert.NotEmpty(t, rec.Header().Get("Retry-After"))
			}
		})
	}
}

