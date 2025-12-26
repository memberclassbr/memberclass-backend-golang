package ports

import "context"

type RateLimiterIP interface {
	CheckLimit(ctx context.Context, ip string) (bool, RateLimitInfo, error)
	Increment(ctx context.Context, ip string) error
}

