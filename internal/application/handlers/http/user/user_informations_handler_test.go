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
	user2 "github.com/memberclass-backend-golang/internal/domain/usecases/user"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserInformationsHandler(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestUserInformationsHandler_GetUserInformations_Success(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	lastAccess := "2024-01-15T10:30:00.000Z"
	users := []user.UserInformation{
		{
			UserID:     "user-123",
			Email:      "test@example.com",
			IsPaid:     true,
			Deliveries: []user.DeliveryInfo{},
			LastAccess: &lastAccess,
		},
	}

	responseData := &user.UserInformationsResponse{
		Users: users,
		Pagination: user.UserInformationsPagination{
			Page:            1,
			TotalPages:      1,
			TotalItems:      1,
			ItemsPerPage:    10,
			HasNextPage:     false,
			HasPreviousPage: false,
		},
	}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user.UserInformationsResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Users))
	assert.Equal(t, "user-123", result.Users[0].UserID)
	assert.Equal(t, "test@example.com", result.Users[0].Email)

	mockUseCase.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_WithEmail(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "test@example.com"

	lastAccess := "2024-01-15T10:30:00.000Z"
	users := []user.UserInformation{
		{
			UserID:     "user-123",
			Email:      email,
			IsPaid:     true,
			Deliveries: []user.DeliveryInfo{},
			LastAccess: &lastAccess,
		},
	}

	responseData := &user.UserInformationsResponse{
		Users: users,
		Pagination: user.UserInformationsPagination{
			Page:            1,
			TotalPages:      1,
			TotalItems:      1,
			ItemsPerPage:    10,
			HasNextPage:     false,
			HasPreviousPage: false,
		},
	}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(responseData, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?email="+email+"&page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user.UserInformationsResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Users))
	assert.Equal(t, email, result.Users[0].Email)

	mockUseCase.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/user/informations", nil)
	w := httptest.NewRecorder()

	handler.GetUserInformations(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", result.Error)
}

func TestUserInformationsHandler_GetUserInformations_InvalidPage(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?page=0", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)
}

func TestUserInformationsHandler_GetUserInformations_InvalidLimit(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?limit=200", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)
}

func TestUserInformationsHandler_GetUserInformations_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations", nil)
	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Unauthorized", result.Error)
	assert.Equal(t, "Tenant not found in context", result.Message)
}

func TestUserInformationsHandler_GetUserInformations_UserNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}
	email := "notfound@example.com"

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(nil, user2.ErrUserNotFoundOrNotInTenantForInformations)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "USER_NOT_FOUND", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_ValidationError(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("page deve ser >= 1"))

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)

	mockUseCase.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "database error", result.Message)

	mockUseCase.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "Internal server error", result.Message)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestUserInformationsHandler_GetUserInformations_EmptyResults(t *testing.T) {
	mockUseCase := mocks.NewMockUserInformationsUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserInformationsHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	emptyResponse := &user.UserInformationsResponse{
		Users: []user.UserInformation{},
		Pagination: user.UserInformationsPagination{
			Page:            1,
			TotalPages:      0,
			TotalItems:      0,
			ItemsPerPage:    10,
			HasNextPage:     false,
			HasPreviousPage: false,
		},
	}

	mockUseCase.On("GetUserInformations", mock.Anything, mock.Anything, tenantID).Return(emptyResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/user/informations?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserInformations(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result user.UserInformationsResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Users))
	assert.Equal(t, 0, result.Pagination.TotalItems)

	mockUseCase.AssertExpectations(t)
}
