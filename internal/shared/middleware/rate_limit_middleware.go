package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/shared/httpx"
)

type RateLimitMiddleware struct {
	rateLimiter ratelimit.UploadLimiter
	logger      logger.Logger
}

func NewRateLimitMiddleware(rateLimiter ratelimit.UploadLimiter, logger logger.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

// rateLimitError is the body attached to a 429 from the upload limiter, so the
// client can show how much of the quota is left and when it resets.
type rateLimitError struct {
	CurrentSize   int64 `json:"current_size"`
	MaxSize       int64 `json:"max_size"`
	RemainingSize int64 `json:"remaining_size"`
	ResetTime     int64 `json:"reset_time"`
}

func (m *RateLimitMiddleware) CheckUploadLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userID := r.Header.Get("user_id")
		if userID == "" {
			m.logger.Error("user_id header is required")
			httpx.WriteError(w, "user_id header is required", http.StatusBadRequest)
			return
		}

		fileSize := r.ContentLength
		if fileSize <= 0 {
			m.logger.Error("Invalid file size")
			httpx.WriteError(w, "Invalid file size", http.StatusBadRequest)
			return
		}

		response, err := m.rateLimiter.CheckUploadLimit(ctx, userID, fileSize)
		if err != nil {
			m.logger.Error("Error checking upload limit: " + err.Error())
			httpx.WriteError(w, "Error checking upload limit", http.StatusInternalServerError)
			return
		}

		if !response.Allowed {
			m.logger.Warn(fmt.Sprintf("Upload limit exceeded for user %s. Current: %d bytes, Requested: %d bytes, Limit: %d bytes",
				userID, response.CurrentSize, fileSize, response.MaxSize))

			rateLimitData := rateLimitError{
				CurrentSize:   response.CurrentSize,
				MaxSize:       response.MaxSize,
				RemainingSize: response.RemainingSize,
				ResetTime:     response.ResetTime,
			}

			httpx.WriteErrorWithData(w, "Upload limit exceeded", rateLimitData, http.StatusTooManyRequests)
			return
		}

		ctx = context.WithValue(ctx, "rate_limit_response", response)
		ctx = context.WithValue(ctx, "user_id", userID)
		ctx = context.WithValue(ctx, "file_size", fileSize)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *RateLimitMiddleware) IncrementAfterUpload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseWriter := &responseWriter{
			ResponseWriter: w,
			statusCode:     200,
		}

		next.ServeHTTP(responseWriter, r)

		if responseWriter.statusCode >= 200 && responseWriter.statusCode < 300 {
			ctx := r.Context()

			userID, ok := ctx.Value("user_id").(string)
			if !ok {
				m.logger.Error("user_id not found in context")
				return
			}

			fileSize, ok := ctx.Value("file_size").(int64)
			if !ok {
				m.logger.Error("file_size not found in context")
				return
			}

			if err := m.rateLimiter.IncrementUploadSize(ctx, userID, fileSize); err != nil {
				m.logger.Error("Error incrementing upload size: " + err.Error())
			} else {
				m.logger.Info(fmt.Sprintf("Incremented upload size for user %s by %d bytes", userID, fileSize))
			}
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
