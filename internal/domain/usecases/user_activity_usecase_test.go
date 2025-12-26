package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserActivityUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	assert.NotNil(t, useCase)
}

func TestUserActivityUseCase_GetUserActivities_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	activities := []response.AccessData{
		{Data: "2025-12-10T10:00:00Z"},
		{Data: "2025-12-10T09:00:00Z"},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserActivityRepo.On("FindActivitiesByEmail", mock.Anything, email, 1, 10).Return(activities, int64(2), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, 2, len(result.Access))
	assert.Equal(t, 1, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)
	assert.Equal(t, 2, result.Pagination.TotalCount)
	assert.Equal(t, 1, result.Pagination.TotalPages)
	assert.False(t, result.Pagination.HasNextPage)
	assert.False(t, result.Pagination.HasPrevPage)

	mockUserRepo.AssertExpectations(t)
	mockUserActivityRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_CacheHit(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	cachedResponse := &response.ActivityResponse{
		Email: email,
		Access: []response.AccessData{
			{Data: "2025-12-10T10:00:00Z"},
		},
		Pagination: response.Pagination{
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

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, 1, len(result.Access))

	mockUserRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
	mockUserActivityRepo.AssertNotCalled(t, "FindActivitiesByEmail")
}

func TestUserActivityUseCase_GetUserActivities_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	req := request.GetActivitiesRequest{
		Email: "",
		Page:  1,
		Limit: 10,
	}

	result, err := useCase.GetUserActivities(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "email é obrigatório", err.Error())
}

func TestUserActivityUseCase_GetUserActivities_UserNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "notfound@example.com"
	tenantID := "tenant-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUserRepo.On("FindByEmail", email).Return(nil, memberclasserrors.ErrUserNotFound)

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenant, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_UserNotInTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(false, nil)

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenant, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_BelongsToTenantError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	repoError := errors.New("database error")
	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(false, repoError)

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockUserRepo.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	repoError := errors.New("database error")

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserActivityRepo.On("FindActivitiesByEmail", mock.Anything, email, 1, 10).Return(nil, int64(0), repoError)

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockUserRepo.AssertExpectations(t)
	mockUserActivityRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_EmptyResult(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	activities := []response.AccessData{}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserActivityRepo.On("FindActivitiesByEmail", mock.Anything, email, 1, 10).Return(activities, int64(0), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, 0, len(result.Access))
	assert.Equal(t, 0, result.Pagination.TotalCount)
	assert.Equal(t, 0, result.Pagination.TotalPages)

	mockUserRepo.AssertExpectations(t)
	mockUserActivityRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  2,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	activities := []response.AccessData{
		{Data: "2025-12-10T10:00:00Z"},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserActivityRepo.On("FindActivitiesByEmail", mock.Anything, email, 2, 10).Return(activities, int64(25), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)
	assert.Equal(t, 25, result.Pagination.TotalCount)
	assert.Equal(t, 3, result.Pagination.TotalPages)
	assert.True(t, result.Pagination.HasNextPage)
	assert.True(t, result.Pagination.HasPrevPage)

	mockUserRepo.AssertExpectations(t)
	mockUserActivityRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestUserActivityUseCase_GetUserActivities_CacheSetError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewUserActivityUseCase(mockLogger, mockUserActivityRepo, mockUserRepo, mockCache)

	email := "test@example.com"
	tenantID := "tenant-123"
	userID := "user-123"

	req := request.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	activities := []response.AccessData{
		{Data: "2025-12-10T10:00:00Z"},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockUserActivityRepo.On("FindActivitiesByEmail", mock.Anything, email, 1, 10).Return(activities, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(errors.New("cache error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetUserActivities(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, email, result.Email)

	mockUserRepo.AssertExpectations(t)
	mockUserActivityRepo.AssertExpectations(t)
	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

