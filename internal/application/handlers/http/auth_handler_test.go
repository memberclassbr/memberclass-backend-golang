package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAuthHandler(t *testing.T) {
	mockUseCase := mocks.NewMockAuthUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewAuthHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestAuthHandler_GenerateMagicLink(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		tenant         *entities.Tenant
		requestBody    interface{}
		mockSetup      func(*mocks.MockAuthUseCase, *mocks.MockLogger)
		expectedStatus int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return method not allowed for GET",
			method: http.MethodGet,
			tenant: &entities.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockAuthUseCase, *mocks.MockLogger) {
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
			name:   "should return unauthorized when tenant is nil",
			method: http.MethodPost,
			tenant: nil,
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(*mocks.MockAuthUseCase, *mocks.MockLogger) {
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
			name:   "should return bad request when body is invalid JSON",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: "invalid json",
			mockSetup: func(*mocks.MockAuthUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Email é obrigatório e deve ser uma string", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:   "should return success when magic link is generated",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(&response.AuthResponse{
					OK:   true,
					Link: "https://example.com/login?token=abc123&email=user@example.com&isReset=false",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response response.AuthResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.OK)
				assert.NotEmpty(t, response.Link)
			},
		},
		{
			name:   "should return bad request when email validation fails",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: ""},
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    400,
					Message: "Email é obrigatório e deve ser uma string",
				})
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Email é obrigatório e deve ser uma string", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:   "should return not found when user not found",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Usuário não encontrado",
				})
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Usuário não encontrado", response["error"])
				assert.Equal(t, "USER_NOT_FOUND", response["errorCode"])
			},
		},
		{
			name:   "should return not found when user not in tenant",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Usuário não encontrado no tenant",
				})
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Usuário não encontrado no tenant", response["error"])
				assert.Equal(t, "USER_NOT_IN_TENANT", response["errorCode"])
			},
		},
		{
			name:   "should return too many requests when rate limit exceeded",
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
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
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(nil, errors.New("unexpected error"))
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
			method: http.MethodPost,
			tenant: &entities.Tenant{ID: "tenant-123"},
			requestBody: map[string]string{
				"email": "user@example.com",
			},
			mockSetup: func(mockUseCase *mocks.MockAuthUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GenerateMagicLink(
					mock.Anything,
					request.AuthRequest{Email: "user@example.com"},
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
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
			mockUseCase := mocks.NewMockAuthUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewAuthHandler(mockUseCase, mockLogger)

			var bodyBytes []byte
			if tt.requestBody != nil {
				if bodyStr, ok := tt.requestBody.(string); ok {
					bodyBytes = []byte(bodyStr)
				} else {
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}

			req := httptest.NewRequest(tt.method, "/api/v1/auth", bytes.NewBuffer(bodyBytes))
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()

			handler.GenerateMagicLink(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

