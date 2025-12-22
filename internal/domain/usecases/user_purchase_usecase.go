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

type UserPurchaseUseCase struct {
	logger   ports.Logger
	userRepo ports.UserRepository
	cache    ports.Cache
}

func NewUserPurchaseUseCase(logger ports.Logger, userRepo ports.UserRepository, cache ports.Cache) ports.UserPurchaseUseCase {
	return &UserPurchaseUseCase{
		logger:   logger,
		userRepo: userRepo,
		cache:    cache,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForPurchases = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *UserPurchaseUseCase) GetUserPurchases(ctx context.Context, req request.GetUserPurchasesRequest, tenantID string) (*response.UserPurchasesResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := uc.userRepo.FindByEmail(req.Email)
	if err != nil {
		return nil, ErrUserNotFoundOrNotInTenantForPurchases
	}

	belongs, err := uc.userRepo.BelongsToTenant(user.ID, tenantID)
	if err != nil {
		return nil, err
	}

	if !belongs {
		return nil, ErrUserNotFoundOrNotInTenantForPurchases
	}

	cacheKey := uc.buildCacheKey(tenantID, req.Email, req.Page, req.Limit, req.Type)

	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cachedResponse response.UserPurchasesResponse
		if err := json.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
			uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &cachedResponse, nil
		}
	}

	purchaseTypes := []string{}
	if req.Type != "" {
		purchaseTypes = []string{req.Type}
	}

	purchases, total, err := uc.userRepo.FindPurchasesByUserAndTenant(ctx, user.ID, tenantID, purchaseTypes, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / req.Limit
	if int(total)%req.Limit > 0 {
		totalPages++
	}

	pagination := response.Pagination{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  int(total),
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	responseData := &response.UserPurchasesResponse{
		Purchases:  purchases,
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

func (uc *UserPurchaseUseCase) buildCacheKey(tenantID, email string, page, limit int, purchaseType string) string {
	return fmt.Sprintf("purchases:%s:%s:%d:%d:%s", tenantID, email, page, limit, purchaseType)
}
