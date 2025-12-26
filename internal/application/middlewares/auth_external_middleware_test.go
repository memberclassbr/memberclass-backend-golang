package middlewares

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockApiTokenUseCase struct {
	mock.Mock
}

func (m *MockApiTokenUseCase) GenerateToken(ctx context.Context, tenantID string) (string, error) {
	args := m.Called(ctx, tenantID)
	return args.String(0), args.Error(1)
}

func (m *MockApiTokenUseCase) ValidateToken(ctx context.Context, token string) (*entities.Tenant, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.Tenant), args.Error(1)
}

func TestNewAuthExternalMiddleware(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	assert.NotNil(t, middleware)
	assert.Equal(t, mockApiTokenUseCase, middleware.ApiTokenUseCase)
}

func TestAuthExternalMiddleware_Authenticate_Success(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	apiKey := "test-api-key"
	hash := generateSHA256Hash(apiKey)
	tenantID := "tenant-123"
	tenant := &entities.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
	}

	mockApiTokenUseCase.On("ValidateToken", mock.Anything, hash).Return(tenant, nil)

	nextHandlerCalled := false
	var contextTenant *entities.Tenant

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		contextTenant = constants.GetTenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("mc-api-key", apiKey)

	rec := httptest.NewRecorder()

	handler := middleware.Authenticate(nextHandler)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, nextHandlerCalled)
	assert.NotNil(t, contextTenant)
	assert.Equal(t, tenantID, contextTenant.ID)

	mockApiTokenUseCase.AssertExpectations(t)
}

func TestAuthExternalMiddleware_Authenticate_MissingApiKey(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	nextHandlerCalled := false

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	rec := httptest.NewRecorder()

	handler := middleware.Authenticate(nextHandler)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, nextHandlerCalled)

	var response map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key invalid", response["error"])
	assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
}

func TestAuthExternalMiddleware_Authenticate_InvalidToken(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	apiKey := "invalid-api-key"
	hash := generateSHA256Hash(apiKey)

	mockApiTokenUseCase.On("ValidateToken", mock.Anything, hash).Return(nil, assert.AnError)

	nextHandlerCalled := false

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("mc-api-key", apiKey)

	rec := httptest.NewRecorder()

	handler := middleware.Authenticate(nextHandler)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, nextHandlerCalled)

	var response map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key invalid", response["error"])

	mockApiTokenUseCase.AssertExpectations(t)
}

func TestAuthExternalMiddleware_Authenticate_EmptyHash(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	nextHandlerCalled := false

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("mc-api-key", "")

	rec := httptest.NewRecorder()

	handler := middleware.Authenticate(nextHandler)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, nextHandlerCalled)

	var response map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
}

func TestAuthExternalMiddleware_generateSHA256Hash(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	input := "test-input"
	expectedHash := sha256.Sum256([]byte(input))
	expectedHashString := hex.EncodeToString(expectedHash[:])

	result := middleware.generateSHA256Hash(input)

	assert.Equal(t, expectedHashString, result)
	assert.NotEmpty(t, result)
	assert.Len(t, result, 64)
}

func TestAuthExternalMiddleware_generateSHA256Hash_EmptyInput(t *testing.T) {
	mockApiTokenUseCase := new(MockApiTokenUseCase)

	middleware := NewAuthExternalMiddleware(mockApiTokenUseCase)

	result := middleware.generateSHA256Hash("")

	assert.NotEmpty(t, result)
	assert.Len(t, result, 64)
}

func TestAuthExternalMiddleware_sendError(t *testing.T) {
	rec := httptest.NewRecorder()

	sendError(rec)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key invalid", response["error"])
	assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
}

func generateSHA256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
