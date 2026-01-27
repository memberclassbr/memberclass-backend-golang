package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	user3 "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type UserInformationsUseCase struct {
	logger   ports.Logger
	userRepo user3.UserRepository
	cache    ports.Cache
}

func NewUserInformationsUseCase(logger ports.Logger, userRepo user3.UserRepository, cache ports.Cache) user3.UserInformationsUseCase {
	return &UserInformationsUseCase{
		logger:   logger,
		userRepo: userRepo,
		cache:    cache,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForInformations = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *UserInformationsUseCase) GetUserInformations(ctx context.Context, req user.GetUserInformationsRequest, tenantID string) (*user2.UserInformationsResponse, error) {
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
		var cachedResponse user2.UserInformationsResponse
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

	pagination := dto.PaginationMeta{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  total,
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	responseData := &user2.UserInformationsResponse{
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
