package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
)

type RateLimitIPMiddleware struct {
	rateLimiter ratelimit.IPLimiter
	logger      logger.Logger
}

func NewRateLimitIPMiddleware(rateLimiter ratelimit.IPLimiter, logger logger.Logger) *RateLimitIPMiddleware {
	return &RateLimitIPMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

func (m *RateLimitIPMiddleware) LimitByIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		ip := m.getClientIP(r)
		if ip == "" {
			next.ServeHTTP(w, r)
			return
		}

		allowed, info, err := m.rateLimiter.CheckLimit(ctx, ip)
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

		if err := m.rateLimiter.Increment(ctx, ip); err != nil {
			m.logger.Error("Error incrementing rate limit: " + err.Error())
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitIPMiddleware) getClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}

	remoteAddr := r.RemoteAddr
	if remoteAddr != "" {
		parts := strings.Split(remoteAddr, ":")
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return ""
}

func (m *RateLimitIPMiddleware) setRateLimitHeaders(w http.ResponseWriter, info ratelimit.Info) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.Reset.Unix(), 10))
	if info.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(info.RetryAfter))
	}
}

func (m *RateLimitIPMiddleware) sendRateLimitError(w http.ResponseWriter, info ratelimit.Info) {
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
