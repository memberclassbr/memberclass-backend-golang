package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	user3 "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type UserActivityUseCase struct {
	logger           ports.Logger
	userActivityRepo user3.UserActivityRepository
	userRepo         user3.UserRepository
	cache            ports.Cache
}

func NewUserActivityUseCase(logger ports.Logger, userActivityRepo user3.UserActivityRepository, userRepo user3.UserRepository, cache ports.Cache) user3.UserActivityUseCase {
	return &UserActivityUseCase{
		logger:           logger,
		userActivityRepo: userActivityRepo,
		userRepo:         userRepo,
		cache:            cache,
	}
}

var (
	ErrUserNotFoundOrNotInTenant = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *UserActivityUseCase) GetUserActivities(ctx context.Context, req user.GetActivitiesRequest, tenantID string) (*user2.ActivityResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := uc.userRepo.FindByEmail(req.Email)
	if err != nil {
		return nil, ErrUserNotFoundOrNotInTenant
	}

	belongs, err := uc.userRepo.BelongsToTenant(user.ID, tenantID)
	if err != nil {
		return nil, err
	}

	if !belongs {
		return nil, ErrUserNotFoundOrNotInTenant
	}

	cacheKey := uc.buildCacheKey(req.Email, req.Page, req.Limit)

	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cachedResponse user2.ActivityResponse
		if err := json.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
			uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &cachedResponse, nil
		}
	}

	activities, total, err := uc.userActivityRepo.FindActivitiesByEmail(ctx, req.Email, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / req.Limit
	if int(total)%req.Limit > 0 {
		totalPages++
	}

	pagination := user2.Pagination{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  int(total),
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	responseData := &user2.ActivityResponse{
		Email:      req.Email,
		Access:     activities,
		Pagination: pagination,
	}

	responseJSON, err := json.Marshal(responseData)
	if err == nil {
		cacheExpiration := 5 * time.Minute
		if err := uc.cache.Set(ctx, cacheKey, string(responseJSON), cacheExpiration); err != nil {
			uc.logger.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			uc.logger.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return responseData, nil
}

func (uc *UserActivityUseCase) buildCacheKey(email string, page, limit int) string {
	return fmt.Sprintf("user_activities:%s:page:%d:limit:%d", email, page, limit)
}
