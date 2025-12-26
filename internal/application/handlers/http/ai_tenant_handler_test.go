package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAITenantHandler(t *testing.T) {
	mockUseCase := mocks.NewMockAITenantUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewAITenantHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestAITenantHandler_GetTenantsWithAIEnabled(t *testing.T) {
	os.Setenv("INTERNAL_AI_API_KEY", "test-api-key")
	defer os.Unsetenv("INTERNAL_AI_API_KEY")

	bunnyLibraryID1 := "library-123"
	bunnyLibraryApiKey1 := "api-key-123"
	bunnyLibraryID2 := "library-456"
	bunnyLibraryApiKey2 := "api-key-456"

	tests := []struct {
		name           string
		method         string
		apiKey         string
		mockSetup      func(*mocks.MockAITenantUseCase, *mocks.MockLogger)
		expectedStatus int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return method not allowed for POST",
			method: http.MethodPost,
			apiKey: "test-api-key",
			mockSetup: func(*mocks.MockAITenantUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusMethodNotAllowed,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Method Not Allowed", response["error"])
				assert.Equal(t, "Method not allowed", response["message"])
			},
		},
		{
			name:   "should return unauthorized when api key is missing",
			method: http.MethodGet,
			apiKey: "",
			mockSetup: func(*mocks.MockAITenantUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:   "should return unauthorized when api key is invalid",
			method: http.MethodGet,
			apiKey: "wrong-key",
			mockSetup: func(*mocks.MockAITenantUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:   "should return success with tenants list",
			method: http.MethodGet,
			apiKey: "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(&response.AITenantsResponse{
					Tenants: []response.AITenantData{
						{
							ID:                 "tenant-1",
							Name:               "Tenant 1",
							AIEnabled:          true,
							BunnyLibraryID:     &bunnyLibraryID1,
							BunnyLibraryApiKey: &bunnyLibraryApiKey1,
						},
						{
							ID:                 "tenant-2",
							Name:               "Tenant 2",
							AIEnabled:          true,
							BunnyLibraryID:     &bunnyLibraryID2,
							BunnyLibraryApiKey: &bunnyLibraryApiKey2,
						},
					},
					Total: 2,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response response.AITenantsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, 2, response.Total)
				assert.Len(t, response.Tenants, 2)
				assert.Equal(t, "tenant-1", response.Tenants[0].ID)
				assert.Equal(t, "Tenant 1", response.Tenants[0].Name)
				assert.True(t, response.Tenants[0].AIEnabled)
			},
		},
		{
			name:   "should return success with empty tenants list",
			method: http.MethodGet,
			apiKey: "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(&response.AITenantsResponse{
					Tenants: []response.AITenantData{},
					Total:   0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response response.AITenantsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, 0, response.Total)
				assert.Empty(t, response.Tenants)
			},
		},
		{
			name:   "should return too many requests when rate limit exceeded",
			method: http.MethodGet,
			apiKey: "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(nil, &memberclasserrors.MemberClassError{
					Code:    429,
					Message: "Muitas requisições. Tente novamente em 5 minutos.",
				})
			},
			expectedStatus: http.StatusTooManyRequests,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Muitas requisições. Tente novamente em 5 minutos.", response["error"])
				assert.Equal(t, "RATE_LIMIT_EXCEEDED", response["errorCode"])
			},
		},
		{
			name:   "should return internal server error when unexpected error occurs",
			method: http.MethodGet,
			apiKey: "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(nil, errors.New("unexpected error"))
				mockLogger.EXPECT().Error("Unexpected error: unexpected error")
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
				assert.Equal(t, "Erro interno do servidor", response["message"])
			},
		},
		{
			name:   "should return error with custom code when member class error has custom code",
			method: http.MethodGet,
			apiKey: "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(nil, &memberclasserrors.MemberClassError{
					Code:    500,
					Message: "Internal error",
				})
			},
			expectedStatus: 500,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
				assert.Equal(t, "Internal error", response["message"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockAITenantUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewAITenantHandler(mockUseCase, mockLogger)

			req := httptest.NewRequest(tt.method, "/api/v1/ai/tenants", nil)
			req.Header.Set("x-internal-api-key", tt.apiKey)

			w := httptest.NewRecorder()

			handler.GetTenantsWithAIEnabled(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

