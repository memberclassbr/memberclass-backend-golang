package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/ports/rate_limit"
	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/constants"
)

type RateLimiterUpload struct {
	client cache.Cache
	log    logger.Logger
}

func NewRateLimiterUpload(client cache.Cache, log logger.Logger) rate_limit.RateLimiterUpload {
	return &RateLimiterUpload{
		client: client,
		log:    log,
	}
}

func (r *RateLimiterUpload) CheckUploadLimit(ctx context.Context, key string, fileSize int64) (dto.RateLimitResponseDTO, error) {
	currentSize, err := r.GetCurrentUploadSize(ctx, key)
	if err != nil {
		r.log.Error("Error getting current upload size for key " + key + ": " + err.Error())
		return dto.RateLimitResponseDTO{}, err
	}

	wouldExceed := currentSize+fileSize > constants.MaxUploadSizePerDay
	remainingSize := constants.MaxUploadSizePerDay - currentSize

	if remainingSize < 0 {
		remainingSize = 0
	}

	return dto.RateLimitResponseDTO{
		Allowed:       !wouldExceed,
		CurrentSize:   currentSize,
		MaxSize:       constants.MaxUploadSizePerDay,
		RemainingSize: remainingSize,
		ResetTime:     time.Now().Add(constants.UploadLimitExpiration).Unix(),
	}, nil
}

func (r *RateLimiterUpload) IncrementUploadSize(ctx context.Context, key string, fileSize int64) error {
	redisKey := constants.UploadLimitKeyPrefix + key

	exists, err := r.client.Exists(ctx, redisKey)
	if err != nil {
		r.log.Error("Error checking if key exists: " + err.Error())
		return err
	}

	currentSize, err := r.client.Increment(ctx, redisKey, fileSize)
	if err != nil {
		r.log.Error("Error incrementing upload size for key " + key + ": " + err.Error())
		return err
	}

	if !exists {
		err = r.client.Set(ctx, redisKey, strconv.FormatInt(currentSize, 10), constants.UploadLimitExpiration)
		if err != nil {
			r.log.Error("Error setting expiration for key " + key + ": " + err.Error())
			return err
		}
	}

	r.log.Info("Incremented upload size for key " + key + " by " + strconv.FormatInt(fileSize, 10) + " bytes")
	return nil
}

func (r *RateLimiterUpload) GetCurrentUploadSize(ctx context.Context, key string) (int64, error) {
	redisKey := constants.UploadLimitKeyPrefix + key

	result, err := r.client.Get(ctx, redisKey)
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0, nil
		}
		return 0, err
	}

	size, err := strconv.ParseInt(result, 10, 64)
	if err != nil {
		r.log.Error("Error parsing upload size for key " + key + ": " + err.Error())
		return 0, err
	}

	return size, nil
}
