package rate_limiter

import (
	"context"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/rate_limit"
)

type RateLimiterIPImpl struct {
	cache ports.Cache
	log   ports.Logger
}

func NewRateLimiterIP(cache ports.Cache, log ports.Logger) rate_limit.RateLimiterIP {
	return &RateLimiterIPImpl{
		cache: cache,
		log:   log,
	}
}

func (r *RateLimiterIPImpl) CheckLimit(ctx context.Context, ip string) (bool, rate_limit.RateLimitInfo, error) {
	key := constants.APIRateLimitIPKeyPrefix + ip
	return r.checkLimit(ctx, key, constants.APIRateLimitIPLimit, constants.APIRateLimitWindow)
}

func (r *RateLimiterIPImpl) Increment(ctx context.Context, ip string) error {
	key := constants.APIRateLimitIPKeyPrefix + ip
	return r.increment(ctx, key, constants.APIRateLimitIPLimit, constants.APIRateLimitWindow)
}

func (r *RateLimiterIPImpl) checkLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, rate_limit.RateLimitInfo, error) {
	current, err := r.cache.Get(ctx, key)
	if err != nil {
		if err.Error() == "redis: nil" {
			return true, rate_limit.RateLimitInfo{
				Limit:      limit,
				Remaining:  limit,
				Reset:      time.Now().Add(window),
				RetryAfter: 0,
			}, nil
		}
		r.log.Error("Error getting rate limit for key " + key + ": " + err.Error())
		return false, rate_limit.RateLimitInfo{}, err
	}

	count, err := strconv.Atoi(current)
	if err != nil {
		r.log.Error("Error parsing rate limit count for key " + key + ": " + err.Error())
		return false, rate_limit.RateLimitInfo{}, err
	}

	ttl, err := r.cache.TTL(ctx, key)
	if err != nil {
		r.log.Error("Error getting TTL for key " + key + ": " + err.Error())
		ttl = window
	}

	resetTime := time.Now().Add(ttl)
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	allowed := count < limit
	retryAfter := 0
	if !allowed {
		retryAfter = int(ttl.Seconds())
	}

	return allowed, rate_limit.RateLimitInfo{
		Limit:      limit,
		Remaining:  remaining,
		Reset:      resetTime,
		RetryAfter: retryAfter,
	}, nil
}

func (r *RateLimiterIPImpl) increment(ctx context.Context, key string, limit int, window time.Duration) error {
	exists, err := r.cache.Exists(ctx, key)
	if err != nil {
		r.log.Error("Error checking if key exists: " + err.Error())
		return err
	}

	count, err := r.cache.Increment(ctx, key, 1)
	if err != nil {
		r.log.Error("Error incrementing rate limit for key " + key + ": " + err.Error())
		return err
	}

	if !exists {
		err = r.cache.Set(ctx, key, strconv.FormatInt(count, 10), window)
		if err != nil {
			r.log.Error("Error setting expiration for key " + key + ": " + err.Error())
			return err
		}
	}

	return nil
}
