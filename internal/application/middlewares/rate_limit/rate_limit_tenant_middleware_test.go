package rate_limit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/ports/rate_limit"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewRateLimitTenantMiddleware(t *testing.T) {
	mockRateLimiter := mocks.NewMockRateLimiterTenant(t)
	mockLogger := mocks.NewMockLogger(t)

	middleware := NewRateLimitTenantMiddleware(mockRateLimiter, mockLogger)

	assert.NotNil(t, middleware)
	assert.Equal(t, mockRateLimiter, middleware.rateLimiter)
	assert.Equal(t, mockLogger, middleware.logger)
}

func TestRateLimitTenantMiddleware_LimitByTenant(t *testing.T) {
	tests := []struct {
		name               string
		tenant             *tenant.Tenant
		endpoint           string
		allowed            bool
		rateLimitInfo      rate_limit.RateLimitInfo
		checkLimitError    error
		incrementError     error
		expectedStatusCode int
		shouldCallNext     bool
		expectHeaders      bool
		expectErrorBody    bool
	}{
		{
			name:     "should allow request when within limit",
			tenant:   &tenant.Tenant{ID: "tenant-123", Name: "Test Tenant"},
			endpoint: "/api/v1/comments",
			allowed:  true,
			rateLimitInfo: rate_limit.RateLimitInfo{
				Limit:      100,
				Remaining:  50,
				Reset:      time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:    nil,
			incrementError:     nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
			expectHeaders:      true,
			expectErrorBody:    false,
		},
		{
			name:     "should deny request when limit exceeded",
			tenant:   &tenant.Tenant{ID: "tenant-123", Name: "Test Tenant"},
			endpoint: "/api/v1/comments",
			allowed:  false,
			rateLimitInfo: rate_limit.RateLimitInfo{
				Limit:      100,
				Remaining:  0,
				Reset:      time.Now().Add(30 * time.Second),
				RetryAfter: 30,
			},
			checkLimitError:    nil,
			incrementError:     nil,
			expectedStatusCode: http.StatusTooManyRequests,
			shouldCallNext:     false,
			expectHeaders:      true,
			expectErrorBody:    true,
		},
		{
			name:               "should pass through when tenant is nil",
			tenant:             nil,
			endpoint:           "/api/v1/comments",
			allowed:            false,
			rateLimitInfo:      rate_limit.RateLimitInfo{},
			checkLimitError:    nil,
			incrementError:     nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
			expectHeaders:      false,
			expectErrorBody:    false,
		},
		{
			name:               "should pass through when check limit fails",
			tenant:             &tenant.Tenant{ID: "tenant-123", Name: "Test Tenant"},
			endpoint:           "/api/v1/comments",
			allowed:            false,
			rateLimitInfo:      rate_limit.RateLimitInfo{},
			checkLimitError:    assert.AnError,
			incrementError:     nil,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
			expectHeaders:      false,
			expectErrorBody:    false,
		},
		{
			name:     "should continue when increment fails",
			tenant:   &tenant.Tenant{ID: "tenant-123", Name: "Test Tenant"},
			endpoint: "/api/v1/comments",
			allowed:  true,
			rateLimitInfo: rate_limit.RateLimitInfo{
				Limit:      100,
				Remaining:  50,
				Reset:      time.Now().Add(30 * time.Second),
				RetryAfter: 0,
			},
			checkLimitError:    nil,
			incrementError:     assert.AnError,
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
			expectHeaders:      true,
			expectErrorBody:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRateLimiter := mocks.NewMockRateLimiterTenant(t)
			mockLogger := mocks.NewMockLogger(t)

			middleware := NewRateLimitTenantMiddleware(mockRateLimiter, mockLogger)

			nextHandlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextHandlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", tt.endpoint, nil)
			ctx := req.Context()

			if tt.tenant != nil {
				ctx = context.WithValue(ctx, constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)

				if tt.checkLimitError == nil {
					mockRateLimiter.EXPECT().CheckLimit(mock.Anything, tt.tenant.ID, tt.endpoint).Return(tt.allowed, tt.rateLimitInfo, nil)
				} else {
					mockRateLimiter.EXPECT().CheckLimit(mock.Anything, tt.tenant.ID, tt.endpoint).Return(false, rate_limit.RateLimitInfo{}, tt.checkLimitError)
					mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
				}

				if tt.allowed && tt.checkLimitError == nil {
					if tt.incrementError != nil {
						mockRateLimiter.EXPECT().Increment(mock.Anything, tt.tenant.ID, tt.endpoint).Return(tt.incrementError)
						mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
					} else {
						mockRateLimiter.EXPECT().Increment(mock.Anything, tt.tenant.ID, tt.endpoint).Return(nil)
					}
				}
			}

			rec := httptest.NewRecorder()

			handler := middleware.LimitByTenant(nextHandler)
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
