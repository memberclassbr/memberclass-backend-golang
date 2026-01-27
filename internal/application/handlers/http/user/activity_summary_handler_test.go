package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewActivitySummaryHandler(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestActivitySummaryHandler_GetActivitySummary_Success(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	lastAccess := "2024-01-15T10:30:00.000Z"
	responseData := &user.ActivitySummaryResponse{
		Users: []user.UserActivitySummary{
			{
				Email:        "user1@example.com",
				UltimoAcesso: &lastAccess,
			},
			{
				Email:        "user2@example.com",
				UltimoAcesso: nil,
			},
		},
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  2,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetActivitySummary", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user.ActivitySummaryResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Users))
	assert.Equal(t, "user1@example.com", result.Users[0].Email)

	mockUseCase.AssertExpectations(t)
}

func TestActivitySummaryHandler_GetActivitySummary_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/user/activity/summary", nil)
	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", result.Error)
}

func TestActivitySummaryHandler_GetActivitySummary_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary", nil)
	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_API_KEY", result["errorCode"])
}

func TestActivitySummaryHandler_GetActivitySummary_InvalidPage(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?page=abc", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_PAGINATION", result["errorCode"])
}

func TestActivitySummaryHandler_GetActivitySummary_InvalidDateRange(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?endDate=2024-01-01T00:00:00Z", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_DATE_RANGE", result["errorCode"])
}

func TestActivitySummaryHandler_GetActivitySummary_WithNoAccess(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	responseData := &user.ActivitySummaryResponse{
		Users: []user.UserActivitySummary{
			{
				Email:        "user1@example.com",
				UltimoAcesso: nil,
			},
		},
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetActivitySummary", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?page=1&limit=10&noAccess=true", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user.ActivitySummaryResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Users))

	mockUseCase.AssertExpectations(t)
}

func TestActivitySummaryHandler_GetActivitySummary_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	mockUseCase.On("GetActivitySummary", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestActivitySummaryHandler_GetActivitySummary_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockActivitySummaryUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewActivitySummaryHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUseCase.On("GetActivitySummary", mock.Anything, mock.Anything, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activity/summary?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetActivitySummary(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "database error", result.Message)

	mockUseCase.AssertExpectations(t)
}
