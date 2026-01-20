package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	purchasesdto "github.com/memberclass-backend-golang/internal/domain/dto/response/purchases"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	tenantentities "github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	user2 "github.com/memberclass-backend-golang/internal/domain/usecases/user"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserPurchaseHandler(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestUserPurchaseHandler_GetUserPurchases_Success(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

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

	purchaseResponse := &purchasesdto.UserPurchasesResponse{
		Purchases: purchases,
		Pagination: activity.Pagination{
			Page:        1,
			Limit:       10,
			TotalCount:  2,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(purchaseResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email+"&page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result purchasesdto.UserPurchasesResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Purchases))
	assert.Equal(t, "event-123", result.Purchases[0].ID)
	assert.Equal(t, "purchase", result.Purchases[0].Type)
	assert.Equal(t, 1, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)

	mockUseCase.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/users/purchases", nil)
	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", result.Error)
	assert.Equal(t, "Method not allowed", result.Message)
}

func TestUserPurchaseHandler_GetUserPurchases_MissingEmail(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "email é obrigatório", result["error"])
	assert.Equal(t, "MISSING_EMAIL", result["errorCode"])
}

func TestUserPurchaseHandler_GetUserPurchases_InvalidPage(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email=test@example.com&page=0", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)
}

func TestUserPurchaseHandler_GetUserPurchases_InvalidLimit(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email=test@example.com&limit=200", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)
}

func TestUserPurchaseHandler_GetUserPurchases_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email=test@example.com", nil)
	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Unauthorized", result.Error)
	assert.Equal(t, "Tenant not found in context", result.Message)
}

func TestUserPurchaseHandler_GetUserPurchases_UserNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "notfound@example.com"

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(nil, user2.ErrUserNotFoundOrNotInTenantForPurchases)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "Usuário não encontrado ou não pertence ao tenant autenticado", result["error"])
	assert.Equal(t, "USER_NOT_FOUND", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_UseCaseValidationError(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("email é obrigatório"))

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "email é obrigatório", result["error"])
	assert.Equal(t, "MISSING_EMAIL", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "database error", result.Message)

	mockUseCase.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email, nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "Internal server error", result.Message)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_EmptyResults(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

	emptyResponse := &purchasesdto.UserPurchasesResponse{
		Purchases: []purchasesdto.UserPurchaseData{},
		Pagination: activity.Pagination{
			Page:        1,
			Limit:       10,
			TotalCount:  0,
			TotalPages:  0,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(emptyResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email+"&page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result purchasesdto.UserPurchasesResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Purchases))
	assert.Equal(t, 0, result.Pagination.TotalCount)

	mockUseCase.AssertExpectations(t)
}

func TestUserPurchaseHandler_GetUserPurchases_WithTypeFilter(t *testing.T) {
	mockUseCase := mocks.NewMockUserPurchaseUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewUserPurchaseHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenantentities.Tenant{ID: tenantID}
	email := "test@example.com"

	purchases := []purchasesdto.UserPurchaseData{
		{
			ID:        "event-123",
			Type:      "purchase",
			CreatedAt: "2024-01-15T10:30:00.000Z",
			UpdatedAt: "2024-01-15T10:30:00.000Z",
		},
	}

	purchaseResponse := &purchasesdto.UserPurchasesResponse{
		Purchases: purchases,
		Pagination: activity.Pagination{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetUserPurchases", mock.Anything, mock.Anything, tenantID).Return(purchaseResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/users/purchases?email="+email+"&type=purchase", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserPurchases(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result purchasesdto.UserPurchasesResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Purchases))
	assert.Equal(t, "purchase", result.Purchases[0].Type)

	mockUseCase.AssertExpectations(t)
}
