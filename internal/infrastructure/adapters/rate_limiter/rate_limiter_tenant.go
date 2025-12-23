package rate_limiter

import (
	"context"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type RateLimiterTenantImpl struct {
	cache ports.Cache
	log   ports.Logger
}

func NewRateLimiterTenant(cache ports.Cache, log ports.Logger) ports.RateLimiterTenant {
	return &RateLimiterTenantImpl{
		cache: cache,
		log:   log,
	}
}

func (r *RateLimiterTenantImpl) CheckLimit(ctx context.Context, tenantID string, endpoint string) (bool, ports.RateLimitInfo, error) {
	key := constants.APIRateLimitTenantKeyPrefix + tenantID + ":" + endpoint
	return r.checkLimit(ctx, key, constants.APIRateLimitTenantLimit, constants.APIRateLimitWindow)
}

func (r *RateLimiterTenantImpl) Increment(ctx context.Context, tenantID string, endpoint string) error {
	key := constants.APIRateLimitTenantKeyPrefix + tenantID + ":" + endpoint
	return r.increment(ctx, key, constants.APIRateLimitTenantLimit, constants.APIRateLimitWindow)
}

func (r *RateLimiterTenantImpl) checkLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, ports.RateLimitInfo, error) {
	current, err := r.cache.Get(ctx, key)
	if err != nil {
		if err.Error() == "redis: nil" {
			return true, ports.RateLimitInfo{
				Limit:      limit,
				Remaining:  limit,
				Reset:      time.Now().Add(window),
				RetryAfter: 0,
			}, nil
		}
		r.log.Error("Error getting rate limit for key " + key + ": " + err.Error())
		return false, ports.RateLimitInfo{}, err
	}

	count, err := strconv.Atoi(current)
	if err != nil {
		r.log.Error("Error parsing rate limit count for key " + key + ": " + err.Error())
		return false, ports.RateLimitInfo{}, err
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

	return allowed, ports.RateLimitInfo{
		Limit:      limit,
		Remaining:  remaining,
		Reset:      resetTime,
		RetryAfter: retryAfter,
	}, nil
}

func (r *RateLimiterTenantImpl) increment(ctx context.Context, key string, limit int, window time.Duration) error {
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

