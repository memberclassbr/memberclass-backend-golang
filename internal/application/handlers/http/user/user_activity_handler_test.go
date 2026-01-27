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
	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	user3 "github.com/memberclass-backend-golang/internal/domain/usecases/user"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserActivityHandler(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestUserActivityHandler_GetUserActivities_Success(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	activities := []user2.AccessData{
		{Data: "2025-12-10T10:00:00Z"},
		{Data: "2025-12-10T09:00:00Z"},
	}

	activityResponse := &user2.ActivityResponse{
		Email:  email,
		Access: activities,
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  2,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(activityResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email+"&page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user2.ActivityResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, 2, len(result.Access))
	assert.Equal(t, "2025-12-10T10:00:00Z", result.Access[0].Data)

	mockUseCase.AssertExpectations(t)
}

func TestUserActivityHandler_GetUserActivities_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/user/activities", nil)
	w := httptest.NewRecorder()

	handler.GetUserActivities(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", response.Error)
}

func TestUserActivityHandler_GetUserActivities_MissingEmail(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/user/activities", nil)
	w := httptest.NewRecorder()

	handler.GetUserActivities(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "email é obrigatório", response["error"])
	assert.Equal(t, "MISSING_EMAIL", response["errorCode"])
}

func TestUserActivityHandler_GetUserActivities_InvalidPage(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/user/activities?email=test@example.com&page=0", nil)
	w := httptest.NewRecorder()

	handler.GetUserActivities(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "page must be a positive integer", response.Message)
}

func TestUserActivityHandler_GetUserActivities_InvalidLimit(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/user/activities?email=test@example.com&limit=101", nil)
	w := httptest.NewRecorder()

	handler.GetUserActivities(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "limit must be between 1 and 100", response.Message)
}

func TestUserActivityHandler_GetUserActivities_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/user/activities?email=test@example.com", nil)
	w := httptest.NewRecorder()

	handler.GetUserActivities(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Tenant not found in context", response.Message)
}

func TestUserActivityHandler_GetUserActivities_UserNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "notfound@example.com"

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(nil, user3.ErrUserNotFoundOrNotInTenant)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Usuário não encontrado ou não pertence ao tenant autenticado", response["error"])
	assert.Equal(t, "USER_NOT_FOUND", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestUserActivityHandler_GetUserActivities_ValidationErrorFromUseCase(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(nil, errors.New("email é obrigatório"))

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "email é obrigatório", response["error"])
	assert.Equal(t, "MISSING_EMAIL", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestUserActivityHandler_GetUserActivities_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    http.StatusInternalServerError,
		Message: "Database error",
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Database error", response.Message)

	mockUseCase.AssertExpectations(t)
}

func TestUserActivityHandler_GetUserActivities_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Internal server error", response.Message)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestUserActivityHandler_GetUserActivities_EmptyActivities(t *testing.T) {
	mockUseCase := mocks.NewMockUserActivityUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserActivityHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	activityResponse := &user2.ActivityResponse{
		Email:  email,
		Access: []user2.AccessData{},
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  0,
			TotalPages:  0,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	req := user.GetActivitiesRequest{
		Email: email,
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetUserActivities", mock.Anything, req, tenantID).Return(activityResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/activities?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserActivities(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user2.ActivityResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, 0, len(result.Access))
	assert.Equal(t, int64(0), result.Pagination.TotalCount)

	mockUseCase.AssertExpectations(t)
}
