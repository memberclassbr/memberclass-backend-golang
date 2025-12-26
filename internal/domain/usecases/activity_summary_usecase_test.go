package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewActivitySummaryUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	assert.NotNil(t, useCase)
}

func TestActivitySummaryUseCase_GetActivitySummary_Success_WithCache(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:  1,
		Limit: 10,
	}

	lastAccess := "2024-01-15T10:30:00.000Z"
	cachedResponse := response.ActivitySummaryResponse{
		Users: []response.UserActivitySummary{
			{
				Email:        "user1@example.com",
				UltimoAcesso: &lastAccess,
			},
		},
		Pagination: response.ActivitySummaryPagination{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	cachedJSON, _ := json.Marshal(cachedResponse)
	cacheKey := "activity:summary:" + tenantID + ":1:10:::false"

	mockCache.On("Get", mock.Anything, cacheKey).Return(string(cachedJSON), nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))

	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_Success_WithActivity(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:  1,
		Limit: 10,
	}

	lastAccess := "2024-01-15T10:30:00.000Z"
	users := []response.UserActivitySummary{
		{
			Email:        "user1@example.com",
			UltimoAcesso: &lastAccess,
		},
	}

	cacheKey := "activity:summary:" + tenantID + ":1:10:::false"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithActivity", mock.Anything, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))
	assert.Equal(t, 1, result.Pagination.TotalCount)

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_Success_WithoutActivity(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:     1,
		Limit:    10,
		NoAccess: true,
	}

	users := []response.UserActivitySummary{
		{
			Email:        "user1@example.com",
			UltimoAcesso: nil,
		},
	}

	cacheKey := "activity:summary:" + tenantID + ":1:10:::true"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithoutActivity", mock.Anything, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))
	assert.Nil(t, result.Users[0].UltimoAcesso)

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_WithDateRange(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	req := request.GetActivitySummaryRequest{
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	users := []response.UserActivitySummary{
		{
			Email:        "user1@example.com",
			UltimoAcesso: nil,
		},
	}

	cacheKey := "activity:summary:" + tenantID + ":1:10:2024-01-01T00:00:00Z:2024-01-31T23:59:59Z:false"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithActivity", mock.Anything, tenantID, startDate, endDate, 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_WithStartDateOnly(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	req := request.GetActivitySummaryRequest{
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
	}

	users := []response.UserActivitySummary{
		{
			Email:        "user1@example.com",
			UltimoAcesso: nil,
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithActivity", mock.Anything, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), 1, 10).Return(users, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Users))

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:  0,
		Limit: 10,
	}

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "page")
}

func TestActivitySummaryUseCase_GetActivitySummary_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:  1,
		Limit: 10,
	}

	cacheKey := "activity:summary:" + tenantID + ":1:10:::false"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithActivity", mock.Anything, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), 1, 10).Return(nil, int64(0), errors.New("database error"))

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "database error", err.Error())

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
}

func TestActivitySummaryUseCase_GetActivitySummary_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockActivityRepo := mocks.NewMockUserActivityRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewActivitySummaryUseCase(mockLogger, mockActivityRepo, mockCache)

	tenantID := "tenant-123"

	req := request.GetActivitySummaryRequest{
		Page:  2,
		Limit: 10,
	}

	users := []response.UserActivitySummary{
		{Email: "user1@example.com"},
		{Email: "user2@example.com"},
	}

	cacheKey := "activity:summary:" + tenantID + ":2:10:::false"

	mockCache.On("Get", mock.Anything, cacheKey).Return("", errors.New("cache miss"))
	mockActivityRepo.On("GetUsersWithActivity", mock.Anything, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), 2, 10).Return(users, int64(25), nil)
	mockCache.On("Set", mock.Anything, cacheKey, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetActivitySummary(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Users))
	assert.Equal(t, 2, result.Pagination.Page)
	assert.Equal(t, 25, result.Pagination.TotalCount)
	assert.Equal(t, 3, result.Pagination.TotalPages)
	assert.True(t, result.Pagination.HasNextPage)
	assert.True(t, result.Pagination.HasPrevPage)

	mockCache.AssertExpectations(t)
	mockActivityRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

