package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	user3 "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type ActivitySummaryUseCase struct {
	logger       ports.Logger
	activityRepo user3.UserActivityRepository
	cache        ports.Cache
}

func NewActivitySummaryUseCase(logger ports.Logger, activityRepo user3.UserActivityRepository, cache ports.Cache) user3.ActivitySummaryUseCase {
	return &ActivitySummaryUseCase{
		logger:       logger,
		activityRepo: activityRepo,
		cache:        cache,
	}
}

func (uc *ActivitySummaryUseCase) GetActivitySummary(ctx context.Context, req user.GetActivitySummaryRequest, tenantID string) (*user2.ActivitySummaryResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := time.Now()
	var startDate, endDate time.Time

	// Determine date range
	if req.StartDate == nil && req.EndDate == nil {
		// Default: last 31 days
		endDate = now
		startDate = now.AddDate(0, 0, -31)
	} else if req.StartDate != nil && req.EndDate == nil {
		// Only startDate: from start of day to end of day
		startDate = time.Date(req.StartDate.Year(), req.StartDate.Month(), req.StartDate.Day(), 0, 0, 0, 0, req.StartDate.Location())
		endDate = time.Date(req.StartDate.Year(), req.StartDate.Month(), req.StartDate.Day(), 23, 59, 59, 999999999, req.StartDate.Location())
	} else {
		// Both dates provided
		startDate = *req.StartDate
		endDate = *req.EndDate
	}

	// Build cache key
	cacheKey := uc.buildCacheKey(tenantID, req, startDate, endDate)

	// Try to get from cache
	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cachedResponse user2.ActivitySummaryResponse
		if err := json.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
			uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &cachedResponse, nil
		}
	}

	var users []user2.UserActivitySummary
	var totalCount int64

	if req.NoAccess {
		users, totalCount, err = uc.activityRepo.GetUsersWithoutActivity(ctx, tenantID, startDate, endDate, req.Page, req.Limit)
	} else {
		users, totalCount, err = uc.activityRepo.GetUsersWithActivity(ctx, tenantID, startDate, endDate, req.Page, req.Limit)
	}

	if err != nil {
		return nil, err
	}

	totalPages := int(totalCount) / req.Limit
	if int(totalCount)%req.Limit > 0 {
		totalPages++
	}

	pagination := user2.ActivitySummaryPagination{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  int(totalCount),
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	responseData := &user2.ActivitySummaryResponse{
		Users:      users,
		Pagination: pagination,
	}

	// Cache the response
	responseJSON, err := json.Marshal(responseData)
	if err == nil {
		cacheExpiration := 300 * time.Second // 5 minutes
		if err := uc.cache.Set(ctx, cacheKey, string(responseJSON), cacheExpiration); err != nil {
			uc.logger.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			uc.logger.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return responseData, nil
}

func (uc *ActivitySummaryUseCase) buildCacheKey(tenantID string, req user.GetActivitySummaryRequest, startDate, endDate time.Time) string {
	startDateStr := ""
	endDateStr := ""
	if req.StartDate != nil {
		startDateStr = startDate.Format(time.RFC3339)
	}
	if req.EndDate != nil {
		endDateStr = endDate.Format(time.RFC3339)
	}
	noAccessStr := "false"
	if req.NoAccess {
		noAccessStr = "true"
	}
	return fmt.Sprintf("activity:summary:%s:%d:%d:%s:%s:%s", tenantID, req.Page, req.Limit, startDateStr, endDateStr, noAccessStr)
}
