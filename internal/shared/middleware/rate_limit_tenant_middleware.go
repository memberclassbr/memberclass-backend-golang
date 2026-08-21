package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

type RateLimitTenantMiddleware struct {
	rateLimiter ratelimit.TenantLimiter
	logger      logger.Logger
}

func NewRateLimitTenantMiddleware(rateLimiter ratelimit.TenantLimiter, logger logger.Logger) *RateLimitTenantMiddleware {
	return &RateLimitTenantMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

func (m *RateLimitTenantMiddleware) LimitByTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// The authenticated tenant wins, and the query string is only read
		// when there is none. The other order — which is what this used to do
		// — keyed the quota on a value the caller writes: a client with a
		// valid API key moved itself to a fresh bucket by sending
		// ?tenantId=anything, which is the same defect the upload limiter had
		// with its user_id header. Rate limiting is per area, and the area is
		// the one the credential resolved to.
		var tenantID string

		if authenticated := tenant.FromContext(ctx); authenticated != nil {
			tenantID = authenticated.ID
		} else if fromQuery := r.URL.Query().Get("tenantId"); fromQuery != "" {
			// Routes mounted without AuthExternal have no tenant in context
			// and still need a bucket; they are already reachable without a
			// credential, so this changes nothing about what they expose.
			tenantID = fromQuery
		} else {
			next.ServeHTTP(w, r)
			return
		}

		endpoint := r.URL.Path
		allowed, info, err := m.rateLimiter.CheckLimit(ctx, tenantID, endpoint)
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

		if err := m.rateLimiter.Increment(ctx, tenantID, endpoint); err != nil {
			m.logger.Error("Error incrementing rate limit: " + err.Error())
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitTenantMiddleware) setRateLimitHeaders(w http.ResponseWriter, info ratelimit.Info) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.Reset.Unix(), 10))
	if info.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(info.RetryAfter))
	}
}

func (m *RateLimitTenantMiddleware) sendRateLimitError(w http.ResponseWriter, info ratelimit.Info) {
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
