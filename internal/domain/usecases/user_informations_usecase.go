package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type UserInformationsUseCase struct {
	logger   ports.Logger
	userRepo ports.UserRepository
	cache    ports.Cache
}

func NewUserInformationsUseCase(logger ports.Logger, userRepo ports.UserRepository, cache ports.Cache) ports.UserInformationsUseCase {
	return &UserInformationsUseCase{
		logger:   logger,
		userRepo: userRepo,
		cache:    cache,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForInformations = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *UserInformationsUseCase) GetUserInformations(ctx context.Context, req request.GetUserInformationsRequest, tenantID string) (*response.UserInformationsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if req.Email != "" {
		user, err := uc.userRepo.FindByEmail(req.Email)
		if err != nil {
			return nil, ErrUserNotFoundOrNotInTenantForInformations
		}

		belongs, err := uc.userRepo.BelongsToTenant(user.ID, tenantID)
		if err != nil {
			return nil, err
		}

		if !belongs {
			return nil, ErrUserNotFoundOrNotInTenantForInformations
		}
	}

	cacheKey := fmt.Sprintf("user:informations:%s:%s:%d:%d", tenantID, req.Email, req.Page, req.Limit)

	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cachedResponse response.UserInformationsResponse
		if err := json.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
			uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &cachedResponse, nil
		}
	}

	users, total, err := uc.userRepo.FindUserInformations(ctx, tenantID, req.Email, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / req.Limit
	if int(total)%req.Limit > 0 {
		totalPages++
	}

	pagination := response.UserInformationsPagination{
		Page:            req.Page,
		TotalPages:      totalPages,
		TotalItems:      int(total),
		ItemsPerPage:    req.Limit,
		HasNextPage:     req.Page < totalPages,
		HasPreviousPage: req.Page > 1,
	}

	responseData := &response.UserInformationsResponse{
		Users:      users,
		Pagination: pagination,
	}

	responseJSON, err := json.Marshal(responseData)
	if err == nil {
		cacheExpiration := 300 * time.Second
		if err := uc.cache.Set(ctx, cacheKey, string(responseJSON), cacheExpiration); err != nil {
			uc.logger.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			uc.logger.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return responseData, nil
}

