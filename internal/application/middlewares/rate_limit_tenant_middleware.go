package middlewares

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type RateLimitTenantMiddleware struct {
	rateLimiter ports.RateLimiterTenant
	logger      ports.Logger
}

func NewRateLimitTenantMiddleware(rateLimiter ports.RateLimiterTenant, logger ports.Logger) *RateLimitTenantMiddleware {
	return &RateLimitTenantMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

func (m *RateLimitTenantMiddleware) LimitByTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tenant := constants.GetTenantFromContext(ctx)
		if tenant == nil {
			next.ServeHTTP(w, r)
			return
		}

		endpoint := r.URL.Path
		allowed, info, err := m.rateLimiter.CheckLimit(ctx, tenant.ID, endpoint)
		if err != nil {
			m.logger.Error("Error checking rate limit: " + err.Error())
			next.ServeHTTP(w, r)
			return
		}

		m.setRateLimitHeaders(w, info)

		if !allowed {
			m.sendRateLimitError(w, info)
			return
		}

		if err := m.rateLimiter.Increment(ctx, tenant.ID, endpoint); err != nil {
			m.logger.Error("Error incrementing rate limit: " + err.Error())
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitTenantMiddleware) setRateLimitHeaders(w http.ResponseWriter, info ports.RateLimitInfo) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.Reset.Unix(), 10))
	if info.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(info.RetryAfter))
	}
}

func (m *RateLimitTenantMiddleware) sendRateLimitError(w http.ResponseWriter, info ports.RateLimitInfo) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.Reset.Unix(), 10))
	w.Header().Set("Retry-After", strconv.Itoa(info.RetryAfter))
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"ok":         false,
		"error":      "Muitas requisições. Tente novamente em " + formatRetryAfter(info.RetryAfter) + ".",
		"errorCode":  "RATE_LIMIT_EXCEEDED",
		"retryAfter": info.RetryAfter,
	}

	json.NewEncoder(w).Encode(response)
}

func formatRetryAfter(seconds int) string {
	minutes := seconds / 60
	if minutes > 0 {
		if minutes == 1 {
			return "1 minuto"
		}
		return strconv.Itoa(minutes) + " minutos"
	}
	if seconds == 1 {
		return "1 segundo"
	}
	return strconv.Itoa(seconds) + " segundos"
}
