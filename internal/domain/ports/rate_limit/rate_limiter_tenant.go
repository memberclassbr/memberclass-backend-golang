package rate_limit

import "context"

type RateLimiterTenant interface {
	CheckLimit(ctx context.Context, tenantID string, endpoint string) (bool, RateLimitInfo, error)
	Increment(ctx context.Context, tenantID string, endpoint string) error
}
