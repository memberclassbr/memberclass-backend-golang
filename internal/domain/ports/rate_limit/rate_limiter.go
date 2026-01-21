package rate_limit

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type RateLimiterUpload interface {
	CheckUploadLimit(ctx context.Context, key string, fileSize int64) (dto.RateLimitResponseDTO, error)
	IncrementUploadSize(ctx context.Context, key string, fileSize int64) error
	GetCurrentUploadSize(ctx context.Context, key string) (int64, error)
}
