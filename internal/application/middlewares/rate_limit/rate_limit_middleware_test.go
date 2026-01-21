package rate_limit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewRateLimitMiddleware(t *testing.T) {
	t.Run("should create new rate limit middleware instance", func(t *testing.T) {
		mockRateLimiter := mocks.NewMockRateLimiterUpload(t)
		mockLogger := mocks.NewMockLogger(t)

		middleware := NewRateLimitMiddleware(mockRateLimiter, mockLogger)

		assert.NotNil(t, middleware)
		assert.Equal(t, mockRateLimiter, middleware.rateLimiter)
		assert.Equal(t, mockLogger, middleware.logger)
	})
}

func TestRateLimitMiddleware_CheckUploadLimit(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		contentLength   int64
		rateLimiterResp dto.RateLimitResponseDTO
		rateLimiterErr  error
		expectedStatus  int
		mockSetup       func(*mocks.MockRateLimiterUpload, *mocks.MockLogger)
	}{
		{
			name:          "should allow upload when within limits",
			userID:        "user-123",
			contentLength: 1000,
			rateLimiterResp: dto.RateLimitResponseDTO{
				Allowed:       true,
				CurrentSize:   500,
				MaxSize:       10000,
				RemainingSize: 9500,
				ResetTime:     time.Now().Add(time.Hour).Unix(),
			},
			expectedStatus: http.StatusOK,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockRL.EXPECT().CheckUploadLimit(mock.Anything, "user-123", int64(1000)).Return(dto.RateLimitResponseDTO{
					Allowed:       true,
					CurrentSize:   500,
					MaxSize:       10000,
					RemainingSize: 9500,
					ResetTime:     time.Now().Add(time.Hour).Unix(),
				}, nil)
			},
		},
		{
			name:          "should reject upload when exceeding limits",
			userID:        "user-123",
			contentLength: 1000,
			rateLimiterResp: dto.RateLimitResponseDTO{
				Allowed:       false,
				CurrentSize:   9500,
				MaxSize:       10000,
				RemainingSize: 500,
				ResetTime:     time.Now().Add(time.Hour).Unix(),
			},
			expectedStatus: http.StatusTooManyRequests,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockRL.EXPECT().CheckUploadLimit(mock.Anything, "user-123", int64(1000)).Return(dto.RateLimitResponseDTO{
					Allowed:       false,
					CurrentSize:   9500,
					MaxSize:       10000,
					RemainingSize: 500,
					ResetTime:     time.Now().Add(time.Hour).Unix(),
				}, nil)
				mockLogger.EXPECT().Warn(mock.Anything).Return()
			},
		},
		{
			name:           "should return error when user_id header missing",
			userID:         "",
			contentLength:  1000,
			expectedStatus: http.StatusBadRequest,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("user_id header is required").Return()
			},
		},
		{
			name:           "should return error when content length invalid",
			userID:         "user-123",
			contentLength:  0,
			expectedStatus: http.StatusBadRequest,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("Invalid file size").Return()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRateLimiter := mocks.NewMockRateLimiterUpload(t)
			mockLogger := mocks.NewMockLogger(t)

			middleware := NewRateLimitMiddleware(mockRateLimiter, mockLogger)

			tt.mockSetup(mockRateLimiter, mockLogger)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("POST", "/test", strings.NewReader("test"))
			req.ContentLength = tt.contentLength
			if tt.userID != "" {
				req.Header.Set("user_id", tt.userID)
			}

			w := httptest.NewRecorder()

			handler := middleware.CheckUploadLimit(nextHandler)
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRateLimitMiddleware_IncrementAfterUpload(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		hasUserID    bool
		hasFileSize  bool
		incrementErr error
		mockSetup    func(*mocks.MockRateLimiterUpload, *mocks.MockLogger)
	}{
		{
			name:        "should increment upload size on success",
			statusCode:  200,
			hasUserID:   true,
			hasFileSize: true,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockRL.EXPECT().IncrementUploadSize(mock.Anything, "user-123", int64(1000)).Return(nil)
				mockLogger.EXPECT().Info(mock.Anything).Return()
			},
		},
		{
			name:        "should not increment on client error",
			statusCode:  400,
			hasUserID:   true,
			hasFileSize: true,
			mockSetup:   func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {},
		},
		{
			name:        "should log error when user_id missing",
			statusCode:  200,
			hasUserID:   false,
			hasFileSize: true,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("user_id not found in context").Return()
			},
		},
		{
			name:        "should log error when file_size missing",
			statusCode:  200,
			hasUserID:   true,
			hasFileSize: false,
			mockSetup: func(mockRL *mocks.MockRateLimiterUpload, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("file_size not found in context").Return()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRateLimiter := mocks.NewMockRateLimiterUpload(t)
			mockLogger := mocks.NewMockLogger(t)

			middleware := NewRateLimitMiddleware(mockRateLimiter, mockLogger)

			tt.mockSetup(mockRateLimiter, mockLogger)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			req := httptest.NewRequest("POST", "/test", nil)
			ctx := req.Context()

			if tt.hasUserID {
				ctx = context.WithValue(ctx, "user_id", "user-123")
			}
			if tt.hasFileSize {
				ctx = context.WithValue(ctx, "file_size", int64(1000))
			}

			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler := middleware.IncrementAfterUpload(nextHandler)
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStatus int
	}{
		{
			name:           "should capture status code 200",
			statusCode:     200,
			expectedStatus: 200,
		},
		{
			name:           "should capture status code 404",
			statusCode:     404,
			expectedStatus: 404,
		},
		{
			name:           "should capture status code 500",
			statusCode:     500,
			expectedStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     200,
			}

			rw.WriteHeader(tt.statusCode)

			assert.Equal(t, tt.expectedStatus, rw.statusCode)
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
