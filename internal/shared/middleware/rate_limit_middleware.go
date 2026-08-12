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

// uploadQuotaKey is the context key under which CheckUploadLimit publishes the
// identity and the byte count IncrementAfterUpload later charges. A typed key
// keeps it from colliding with anything another middleware stores.
type uploadQuotaKey struct{}

// uploadQuota is what the two halves of the limiter pass between them.
type uploadQuota struct {
	userID   string
	fileSize int64
}

// CheckUploadLimit charges the upload against the caller's byte quota.
//
// The identity comes from the verified go-token — `sub`, put on the context by
// BearerMiddleware.RequireAuth — and from nowhere else. It used to be read off
// a `user_id` request header, which the caller writes: a client that wanted an
// empty quota only had to send a different value on every upload, and the
// header was mandatory on a route where the token already names the user.
//
// So this middleware only works below RequireAuth. Mounted without it there is
// no identity to key the quota on, and the request is rejected rather than
// charged to some default bucket.
func (m *RateLimitMiddleware) CheckUploadLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		user := GetAuthUser(ctx)
		if user == nil || user.UserID == "" {
			m.logger.Error("upload limit: no authenticated user on the context; mount CheckUploadLimit below RequireAuth")
			httpx.WriteError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := user.UserID

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

		ctx = context.WithValue(ctx, uploadQuotaKey{}, uploadQuota{userID: userID, fileSize: fileSize})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// IncrementAfterUpload charges the quota once the handler below it has
// answered 2xx, using the identity CheckUploadLimit resolved from the token.
// Mount it directly under CheckUploadLimit; on its own it has nothing to
// charge and quietly does nothing.
func (m *RateLimitMiddleware) IncrementAfterUpload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseWriter := &responseWriter{
			ResponseWriter: w,
			statusCode:     200,
		}

		next.ServeHTTP(responseWriter, r)

		if responseWriter.statusCode >= 200 && responseWriter.statusCode < 300 {
			ctx := r.Context()

			quota, ok := ctx.Value(uploadQuotaKey{}).(uploadQuota)
			if !ok {
				m.logger.Error("upload quota not found in context; mount IncrementAfterUpload below CheckUploadLimit")
				return
			}

			if err := m.rateLimiter.IncrementUploadSize(ctx, quota.userID, quota.fileSize); err != nil {
				m.logger.Error("Error incrementing upload size: " + err.Error())
			} else {
				m.logger.Info(fmt.Sprintf("Incremented upload size for user %s by %d bytes", quota.userID, quota.fileSize))
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
