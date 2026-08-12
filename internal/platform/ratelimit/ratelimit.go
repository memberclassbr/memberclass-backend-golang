package ratelimit

import (
	"context"
	"time"
)

// Info describes the state of a counter at the moment it was checked. The
// middlewares turn it into the X-RateLimit-* response headers.
type Info struct {
	Limit      int
	Remaining  int
	Reset      time.Time
	RetryAfter int
}

// UploadResult is the outcome of an upload-size check. Uploads are limited by
// total bytes in a window rather than by request count, so this carries sizes
// rather than a request tally.
type UploadResult struct {
	Allowed       bool  `json:"allowed"`
	CurrentSize   int64 `json:"current_size"`
	MaxSize       int64 `json:"max_size"`
	RemainingSize int64 `json:"remaining_size"`
	ResetTime     int64 `json:"reset_time"`
}

// TenantLimiter caps requests per tenant per endpoint.
type TenantLimiter interface {
	CheckLimit(ctx context.Context, tenantID string, endpoint string) (bool, Info, error)
	Increment(ctx context.Context, tenantID string, endpoint string) error
}

// IPLimiter caps requests per client IP. It guards endpoints where the caller
// is not yet identified, or where a leaked credential should not buy unbounded
// throughput.
type IPLimiter interface {
	CheckLimit(ctx context.Context, ip string) (bool, Info, error)
	Increment(ctx context.Context, ip string) error
}

// UploadLimiter caps the total bytes one key may upload in a window.
type UploadLimiter interface {
	CheckUploadLimit(ctx context.Context, key string, fileSize int64) (UploadResult, error)
	IncrementUploadSize(ctx context.Context, key string, fileSize int64) error
	GetCurrentUploadSize(ctx context.Context, key string) (int64, error)
}
