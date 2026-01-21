package user

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/purchase"
	purchasesdto "github.com/memberclass-backend-golang/internal/domain/dto/response/purchases"
	useractivitydto "github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	userentities "github.com/memberclass-backend-golang/internal/domain/entities/user"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserPurchaseUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	assert.NotNil(t, useCase)
}

func TestUserPurchaseUseCase_GetUserPurchases_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
		Type:  "",
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	purchases := []purchasesdto.UserPurchaseData{
		{
			ID:        "event-123",
			Type:      "purchase",
			CreatedAt: "2024-01-15T10:30:00.000Z",
			UpdatedAt: "2024-01-15T10:30:00.000Z",
		},
		{
			ID:        "event-456",
			Type:      "refund",
			CreatedAt: "2024-01-14T15:20:00.000Z",
			UpdatedAt: "2024-01-14T15:20:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, mock.Anything, 1, 10).Return(purchases, int64(2), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Purchases))
	assert.Equal(t, "event-123", result.Purchases[0].ID)
	assert.Equal(t, "purchase", result.Purchases[0].Type)
	assert.Equal(t, 1, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)
	assert.Equal(t, 2, result.Pagination.TotalCount)
	assert.Equal(t, 1, result.Pagination.TotalPages)
	assert.False(t, result.Pagination.HasNextPage)
	assert.False(t, result.Pagination.HasPrevPage)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_CacheHit(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
		Type:  "",
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	cachedResponse := purchasesdto.UserPurchasesResponse{
		Purchases: []purchasesdto.UserPurchaseData{
			{
				ID:        "event-123",
				Type:      "purchase",
				CreatedAt: "2024-01-15T10:30:00.000Z",
				UpdatedAt: "2024-01-15T10:30:00.000Z",
			},
		},
		Pagination: useractivitydto.Pagination{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	cachedJSON, _ := json.Marshal(cachedResponse)

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(string(cachedJSON), nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Purchases))
	assert.Equal(t, "event-123", result.Purchases[0].ID)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	req := purchase.GetUserPurchasesRequest{
		Email: "",
		Page:  1,
		Limit: 10,
	}

	result, err := useCase.GetUserPurchases(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "email é obrigatório", err.Error())
}

func TestUserPurchaseUseCase_GetUserPurchases_UserNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "notfound@example.com"
	tenantID := "tenant-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUserRepo.On("FindByEmail", email).Return(nil, memberclasserrors.ErrUserNotFound)

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForPurchases, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_UserNotInTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(false, nil)

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForPurchases, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	repoError := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, mock.Anything, 1, 10).Return(nil, int64(0), repoError)

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_EmptyResults(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, mock.Anything, 1, 10).Return([]purchasesdto.UserPurchaseData{}, int64(0), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Purchases))
	assert.Equal(t, 0, result.Pagination.TotalCount)
	assert.Equal(t, 0, result.Pagination.TotalPages)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_WithTypeFilter(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
		Type:  "purchase",
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	purchases := []purchasesdto.UserPurchaseData{
		{
			ID:        "event-123",
			Type:      "purchase",
			CreatedAt: "2024-01-15T10:30:00.000Z",
			UpdatedAt: "2024-01-15T10:30:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, []string{"purchase"}, 1, 10).Return(purchases, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Purchases))
	assert.Equal(t, "purchase", result.Purchases[0].Type)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  2,
		Limit: 10,
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	purchases := []purchasesdto.UserPurchaseData{
		{
			ID:        "event-11",
			Type:      "purchase",
			CreatedAt: "2024-01-15T10:30:00.000Z",
			UpdatedAt: "2024-01-15T10:30:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, mock.Anything, 2, 10).Return(purchases, int64(25), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)
	assert.Equal(t, 25, result.Pagination.TotalCount)
	assert.Equal(t, 3, result.Pagination.TotalPages)
	assert.True(t, result.Pagination.HasNextPage)
	assert.True(t, result.Pagination.HasPrevPage)

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserPurchaseUseCase_GetUserPurchases_CacheSetError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserPurchaseUseCase(mockLogger, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := purchase.GetUserPurchasesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &userentities.User{
		ID:    userID,
		Email: email,
	}

	purchases := []purchasesdto.UserPurchaseData{
		{
			ID:        "event-123",
			Type:      "purchase",
			CreatedAt: "2024-01-15T10:30:00.000Z",
			UpdatedAt: "2024-01-15T10:30:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindPurchasesByUserAndTenant", mock.Anything, userID, tenantID, mock.Anything, 1, 10).Return(purchases, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(errors.New("cache error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserPurchases(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Purchases))

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}
