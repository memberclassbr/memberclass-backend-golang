package user

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	user3 "github.com/memberclass-backend-golang/internal/domain/entities/user"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserInformationsUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	assert.NotNil(t, useCase)
}

func TestUserInformationsUseCase_GetUserInformations_Success_WithCache(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := user.GetUserInformationsRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &user3.User{
		ID:    userID,
		Email: email,
	}

	users := []user2.UserInformation{
		{
			UserID:     userID,
			Email:      email,
			IsPaid:     true,
			Deliveries: []user2.DeliveryInfo{},
		},
	}

	cachedResponse := user2.UserInformationsResponse{
		Users: users,
		Pagination: user2.UserInformationsPagination{
			Page:            1,
			TotalPages:      1,
			TotalItems:      1,
			ItemsPerPage:    10,
			HasNextPage:     false,
			HasPreviousPage: false,
		},
	}

	cachedJSON, _ := json.Marshal(cachedResponse)
	cacheKey := "user:informations:" + tenantID + ":" + email + ":1:10"

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, cacheKey).Return(string(cachedJSON), nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))
	assert.Equal(t, email, result.Users[0].Email)

	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_Success_WithoutEmail(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"

	req := user.GetUserInformationsRequest{
		Email: "",
		Page:  1,
		Limit: 10,
	}

	users := []user2.UserInformation{
		{
			UserID:     "user-123",
			Email:      "test@example.com",
			IsPaid:     true,
			Deliveries: []user2.DeliveryInfo{},
		},
	}

	cacheKey := "user:informations:" + tenantID + "::1:10"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindUserInformations", mock.Anything, tenantID, "", 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))
	assert.Equal(t, 1, result.Pagination.TotalItems)

	mockCache.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_Success_WithEmail_UserBelongsToTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := user.GetUserInformationsRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &user3.User{
		ID:    userID,
		Email: email,
	}

	users := []user2.UserInformation{
		{
			UserID:     userID,
			Email:      email,
			IsPaid:     true,
			Deliveries: []user2.DeliveryInfo{},
		},
	}

	cacheKey := "user:informations:" + tenantID + ":" + email + ":1:10"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockUserRepo.On("FindUserInformations", mock.Anything, tenantID, email, 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))

	mockCache.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_UserNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"
	email := "notfound@example.com"

	req := user.GetUserInformationsRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUserRepo.On("FindByEmail", email).Return(nil, errors.New("user not found"))

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForInformations, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_UserNotInTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := user.GetUserInformationsRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &user3.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(false, nil)

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForInformations, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"

	req := user.GetUserInformationsRequest{
		Email: "",
		Page:  0,
		Limit: 10,
	}

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "page")
}

func TestUserInformationsUseCase_GetUserInformations_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"

	req := user.GetUserInformationsRequest{
		Email: "",
		Page:  1,
		Limit: 10,
	}

	cacheKey := "user:informations:" + tenantID + "::1:10"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindUserInformations", mock.Anything, tenantID, "", 1, 10).Return(nil, int64(0), errors.New("database error"))

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "database error", err.Error())

	mockCache.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

func TestUserInformationsUseCase_GetUserInformations_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserInformationsUseCase(mockLogger, mockUserRepo, mockCache)

	tenantID := "tenant-123"

	req := user.GetUserInformationsRequest{
		Email: "",
		Page:  2,
		Limit: 10,
	}

	users := []user2.UserInformation{
		{UserID: "user-1", Email: "user1@example.com"},
		{UserID: "user-2", Email: "user2@example.com"},
	}

	cacheKey := "user:informations:" + tenantID + "::2:10"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockUserRepo.On("FindUserInformations", mock.Anything, tenantID, "", 2, 10).Return(users, int64(25), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserInformations(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Users))
	assert.Equal(t, 2, result.Pagination.Page)
	assert.Equal(t, 25, result.Pagination.TotalItems)
	assert.Equal(t, 3, result.Pagination.TotalPages)
	assert.True(t, result.Pagination.HasNextPage)
	assert.True(t, result.Pagination.HasPreviousPage)

	mockCache.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}
