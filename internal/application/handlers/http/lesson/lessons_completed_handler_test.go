package lesson

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/usecases/lessons"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewLessonsCompletedHandler(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_Success(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	responseData := &lesson.LessonsCompletedResponse{
		OK: true,
		Data: lesson.LessonsCompletedData{
			CompletedLessons: []lesson.CompletedLesson{
				{
					CourseName:  "Curso de JavaScript",
					LessonName:  "Introdução ao JS",
					CompletedAt: "2025-12-10T14:30:00.000Z",
				},
			},
			Pagination: activity.Pagination{
				Page:        1,
				Limit:       10,
				TotalCount:  1,
				TotalPages:  1,
				HasNextPage: false,
				HasPrevPage: false,
			},
		},
	}

	mockUseCase.On("GetLessonsCompleted", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email="+email+"&page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result lesson.LessonsCompletedResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.True(t, result.OK)
	assert.Equal(t, 1, len(result.Data.CompletedLessons))
	assert.Equal(t, "Curso de JavaScript", result.Data.CompletedLessons[0].CourseName)

	mockUseCase.AssertExpectations(t)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/user/lessons/completed", nil)
	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", result.Error)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email=test@example.com", nil)
	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_API_KEY", result["errorCode"])
}

func TestLessonsCompletedHandler_GetLessonsCompleted_InvalidPage(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email=test@example.com&page=abc", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_PAGINATION", result["errorCode"])
}

func TestLessonsCompletedHandler_GetLessonsCompleted_MissingEmail(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "MISSING_EMAIL", result["errorCode"])
}

func TestLessonsCompletedHandler_GetLessonsCompleted_InvalidDateRange(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email=test@example.com&endDate=2024-01-01T00:00:00Z", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_DATE_RANGE", result["errorCode"])
}

func TestLessonsCompletedHandler_GetLessonsCompleted_UserNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "notfound@example.com"

	mockUseCase.On("GetLessonsCompleted", mock.Anything, mock.Anything, tenantID).Return(nil, lessons.ErrUserNotFoundOrNotInTenantForLessons)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "USER_NOT_IN_TENANT", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	mockUseCase.On("GetLessonsCompleted", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUseCase.On("GetLessonsCompleted", mock.Anything, mock.Anything, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "database error", result.Message)

	mockUseCase.AssertExpectations(t)
}

func TestLessonsCompletedHandler_GetLessonsCompleted_WithDateRange(t *testing.T) {
	mockUseCase := mocks.NewMockLessonsCompletedUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewLessonsCompletedHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	responseData := &lesson.LessonsCompletedResponse{
		OK: true,
		Data: lesson.LessonsCompletedData{
			CompletedLessons: []lesson.CompletedLesson{},
			Pagination: activity.Pagination{
				Page:        1,
				Limit:       10,
				TotalCount:  0,
				TotalPages:  0,
				HasNextPage: false,
				HasPrevPage: false,
			},
		},
	}

	mockUseCase.On("GetLessonsCompleted", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/lessons/completed?email="+email+"&startDate=2024-01-01T00:00:00Z&endDate=2024-01-31T23:59:59Z", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetLessonsCompleted(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result lesson.LessonsCompletedResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.True(t, result.OK)

	mockUseCase.AssertExpectations(t)
}
