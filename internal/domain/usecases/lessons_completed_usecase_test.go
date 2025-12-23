package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewLessonsCompletedUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	assert.NotNil(t, useCase)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := request.GetLessonsCompletedRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	lessons := []response.CompletedLesson{
		{
			CourseName:  "Curso de JavaScript",
			LessonName:  "Introdução ao JS",
			CompletedAt: "2025-12-10T14:30:00.000Z",
		},
		{
			CourseName:  "Curso de JavaScript",
			LessonName:  "Variáveis e Tipos",
			CompletedAt: "2025-12-09T10:15:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), "", 1, 10).Return(lessons, int64(2), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, 2, len(result.Data.CompletedLessons))
	assert.Equal(t, "Curso de JavaScript", result.Data.CompletedLessons[0].CourseName)
	assert.Equal(t, 1, result.Data.Pagination.Page)
	assert.Equal(t, 2, result.Data.Pagination.TotalCount)

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_WithDateRange(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	req := request.GetLessonsCompletedRequest{
		Email:     email,
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	lessons := []response.CompletedLesson{
		{
			CourseName:  "Curso de JavaScript",
			LessonName:  "Introdução ao JS",
			CompletedAt: "2024-01-15T14:30:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, startDate, endDate, "", 1, 10).Return(lessons, int64(1), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Data.CompletedLessons))

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_WithCourseID(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"
	courseID := "course-123"

	req := request.GetLessonsCompletedRequest{
		Email:    email,
		Page:     1,
		Limit:    10,
		CourseID: courseID,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	lessons := []response.CompletedLesson{
		{
			CourseName:  "Curso de JavaScript",
			LessonName:  "Introdução ao JS",
			CompletedAt: "2025-12-10T14:30:00.000Z",
		},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), courseID, 1, 10).Return(lessons, int64(1), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Data.CompletedLessons))

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_WithStartDateOnly(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	req := request.GetLessonsCompletedRequest{
		Email:     email,
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	lessons := []response.CompletedLesson{}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), "", 1, 10).Return(lessons, int64(0), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Data.CompletedLessons))

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_UserNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "notfound@example.com"

	req := request.GetLessonsCompletedRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUserRepo.On("FindByEmail", email).Return(nil, errors.New("user not found"))

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForLessons, err)

	mockUserRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_UserNotInTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := request.GetLessonsCompletedRequest{
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

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForLessons, err)

	mockUserRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"

	req := request.GetLessonsCompletedRequest{
		Email: "",
		Page:  1,
		Limit: 10,
	}

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "email")
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := request.GetLessonsCompletedRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), "", 1, 10).Return(nil, int64(0), errors.New("database error"))

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "database error", err.Error())

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := request.GetLessonsCompletedRequest{
		Email: email,
		Page:  2,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	lessons := []response.CompletedLesson{
		{CourseName: "Course 1", LessonName: "Lesson 1"},
		{CourseName: "Course 2", LessonName: "Lesson 2"},
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), "", 2, 10).Return(lessons, int64(25), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Data.CompletedLessons))
	assert.Equal(t, 2, result.Data.Pagination.Page)
	assert.Equal(t, 25, result.Data.Pagination.TotalCount)
	assert.Equal(t, 3, result.Data.Pagination.TotalPages)
	assert.True(t, result.Data.Pagination.HasNextPage)
	assert.True(t, result.Data.Pagination.HasPrevPage)

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

func TestLessonsCompletedUseCase_GetLessonsCompleted_EmptyResults(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockLessonRepo := mocks.NewMockLessonRepository(t)

	useCase := NewLessonsCompletedUseCase(mockLogger, mockUserRepo, mockLessonRepo)

	tenantID := "tenant-123"
	email := "test@example.com"
	userID := "user-123"

	req := request.GetLessonsCompletedRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	user := &entities.User{
		ID:    userID,
		Email: email,
	}

	mockUserRepo.On("FindByEmail", email).Return(user, nil)
	mockUserRepo.On("BelongsToTenant", userID, tenantID).Return(true, nil)
	mockLessonRepo.On("FindCompletedLessonsByEmail", mock.Anything, userID, tenantID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time"), "", 1, 10).Return([]response.CompletedLesson{}, int64(0), nil)

	result, err := useCase.GetLessonsCompleted(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, 0, len(result.Data.CompletedLessons))
	assert.Equal(t, 0, result.Data.Pagination.TotalCount)

	mockUserRepo.AssertExpectations(t)
	mockLessonRepo.AssertExpectations(t)
}

