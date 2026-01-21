package auth

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/hkdf"
)

const testSecret = "WLcglAhaob5u0K8heuOSK7rONH7bdF"

func deriveTestEncryptionKey(secret string) ([]byte, error) {
	hash := sha256.New
	info := []byte("NextAuth.js Generated Encryption Key")
	salt := make([]byte, 0)

	reader := hkdf.New(hash, []byte(secret), salt, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func createTestToken(payload dto.SessionPayload, secret string) (string, error) {
	key, err := deriveTestEncryptionKey(secret)
	if err != nil {
		return "", err
	}

	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       key,
		},
		(&jose.EncrypterOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	jwe, err := encrypter.Encrypt(payloadBytes)
	if err != nil {
		return "", err
	}

	return jwe.CompactSerialize()
}

func createValidPayload() dto.SessionPayload {
	return dto.SessionPayload{
		Sub:   "user-123",
		Name:  "Test User",
		Email: "test@example.com",
		Image: "https://example.com/image.jpg",
		Iat:   time.Now().Unix(),
		Exp:   time.Now().Add(24 * time.Hour).Unix(),
		Jti:   "jwt-id-123",
		Role:  "user",
		User: dto.UserInfo{
			ID:       "user-123",
			Email:    "test@example.com",
			Name:     "Test User",
			Username: "testuser",
			Image:    "https://example.com/image.jpg",
			Tenants:  []dto.UserTenant{},
		},
	}
}

func createExpiredPayload() dto.SessionPayload {
	payload := createValidPayload()
	payload.Exp = time.Now().Add(-1 * time.Hour).Unix()
	return payload
}

func TestAuthMiddleware_Authenticate(t *testing.T) {
	tests := []struct {
		name               string
		setupRequest       func(*http.Request) *http.Request
		setupMocks         func(*mocks.MockLogger, *mocks.MockSessionValidatorUseCase)
		expectedStatusCode int
		expectedError      string
		expectedMessage    string
		shouldCallNext     bool
	}{
		{
			name: "should return 401 when cookie is not present",
			setupRequest: func(r *http.Request) *http.Request {
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				logger.On("Debug", mock.Anything, mock.Anything).Maybe()
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Unauthorized",
			expectedMessage:    "Session token not found",
			shouldCallNext:     false,
		},
		{
			name: "should return 401 when token is invalid",
			setupRequest: func(r *http.Request) *http.Request {
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: "invalid-token",
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				logger.On("Error", mock.Anything, mock.Anything).Maybe()
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Unauthorized",
			expectedMessage:    "Invalid session token",
			shouldCallNext:     false,
		},
		{
			name: "should return 401 when token is expired",
			setupRequest: func(r *http.Request) *http.Request {
				payload := createExpiredPayload()
				token, _ := createTestToken(payload, testSecret)
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: token,
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				logger.On("Debug", mock.Anything, mock.Anything).Maybe()
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Unauthorized",
			expectedMessage:    "Session expired",
			shouldCallNext:     false,
		},
		{
			name: "should return 500 when usecase returns internal error",
			setupRequest: func(r *http.Request) *http.Request {
				payload := createValidPayload()
				token, _ := createTestToken(payload, testSecret)
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: token,
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				sessionValidator.On("ValidateUserExists", "user-123").Return(errors.New("database error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedError:      "Internal Server Error",
			expectedMessage:    "Failed to validate user",
			shouldCallNext:     false,
		},
		{
			name: "should return 404 when user does not exist",
			setupRequest: func(r *http.Request) *http.Request {
				payload := createValidPayload()
				token, _ := createTestToken(payload, testSecret)
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: token,
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				sessionValidator.On("ValidateUserExists", "user-123").Return(memberclasserrors.ErrUserNotFound)
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "Not Found",
			expectedMessage:    "user not found",
			shouldCallNext:     false,
		},
		{
			name: "should call next handler when authentication succeeds",
			setupRequest: func(r *http.Request) *http.Request {
				payload := createValidPayload()
				token, _ := createTestToken(payload, testSecret)
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: token,
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				sessionValidator.On("ValidateUserExists", "user-123").Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name: "should pass user data in context when authentication succeeds",
			setupRequest: func(r *http.Request) *http.Request {
				payload := createValidPayload()
				token, _ := createTestToken(payload, testSecret)
				r.AddCookie(&http.Cookie{
					Name:  "next-auth.session-token",
					Value: token,
				})
				return r
			},
			setupMocks: func(logger *mocks.MockLogger, sessionValidator *mocks.MockSessionValidatorUseCase) {
				sessionValidator.On("ValidateUserExists", "user-123").Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

			tt.setupMocks(mockLogger, mockSessionValidator)

			middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

			nextHandlerCalled := false
			var contextUser *dto.SessionPayload

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextHandlerCalled = true
				contextUser = dto.GetUserFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req = tt.setupRequest(req)

			rec := httptest.NewRecorder()

			handler := middleware.Authenticate(nextHandler)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)
			assert.Equal(t, tt.shouldCallNext, nextHandlerCalled)

			if !tt.shouldCallNext {
				var response map[string]string
				err := json.NewDecoder(rec.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedError, response["error"])
				assert.Equal(t, tt.expectedMessage, response["message"])
			}

			if tt.shouldCallNext && tt.name == "should pass user data in context when authentication succeeds" {
				assert.NotNil(t, contextUser)
				assert.Equal(t, "user-123", contextUser.User.ID)
				assert.Equal(t, "test@example.com", contextUser.User.Email)
				assert.Equal(t, "Test User", contextUser.User.Name)
			}
		})
	}
}

func TestAuthMiddleware_DeriveEncryptionKey(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

	middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

	key, err := middleware.deriveEncryptionKey()

	assert.NoError(t, err)
	assert.NotNil(t, key)
	assert.Len(t, key, 32)
}

func TestAuthMiddleware_DecryptToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		expectError   bool
		expectedEmail string
	}{
		{
			name:        "should return error for invalid token format",
			token:       "invalid-token",
			expectError: true,
		},
		{
			name:        "should return error for malformed JWE",
			token:       "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

			middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

			payload, err := middleware.decryptToken(tt.token)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, payload)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, payload)
				assert.Equal(t, tt.expectedEmail, payload.Email)
			}
		})
	}
}

func TestAuthMiddleware_DecryptToken_ValidToken(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

	middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

	expectedPayload := createValidPayload()
	token, err := createTestToken(expectedPayload, testSecret)
	assert.NoError(t, err)

	payload, err := middleware.decryptToken(token)

	assert.NoError(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, expectedPayload.Email, payload.Email)
	assert.Equal(t, expectedPayload.User.ID, payload.User.ID)
	assert.Equal(t, expectedPayload.User.Name, payload.User.Name)
}

func TestAuthMiddleware_SendError(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		message         string
		expectedError   string
		expectedMessage string
	}{
		{
			name:            "should send 401 error correctly",
			statusCode:      http.StatusUnauthorized,
			message:         "Test unauthorized",
			expectedError:   "Unauthorized",
			expectedMessage: "Test unauthorized",
		},
		{
			name:            "should send 500 error correctly",
			statusCode:      http.StatusInternalServerError,
			message:         "Test server error",
			expectedError:   "Internal Server Error",
			expectedMessage: "Test server error",
		},
		{
			name:            "should send 403 error correctly",
			statusCode:      http.StatusForbidden,
			message:         "Test forbidden",
			expectedError:   "Forbidden",
			expectedMessage: "Test forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

			middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

			rec := httptest.NewRecorder()
			middleware.sendError(rec, tt.statusCode, tt.message)

			assert.Equal(t, tt.statusCode, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var response map[string]string
			err := json.NewDecoder(rec.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedError, response["error"])
			assert.Equal(t, tt.expectedMessage, response["message"])
		})
	}
}

func TestNewAuthMiddleware(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

	middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.logger)
	assert.NotNil(t, middleware.sessionValidator)
	assert.NotNil(t, middleware.secret)
}

func TestAuthMiddleware_TokenWithoutExpiration(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

	mockSessionValidator.On("ValidateUserExists", "user-123").Return(nil)

	middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

	payload := createValidPayload()
	payload.Exp = 0
	token, err := createTestToken(payload, testSecret)
	assert.NoError(t, err)

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "next-auth.session-token",
		Value: token,
	})

	rec := httptest.NewRecorder()

	handler := middleware.Authenticate(nextHandler)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, nextHandlerCalled)
}

func TestAuthMiddleware_HandleValidationError(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		expectedStatusCode int
		expectedMessage    string
	}{
		{
			name:               "should handle MemberClassError correctly",
			err:                memberclasserrors.ErrUserNotFound,
			expectedStatusCode: http.StatusNotFound,
			expectedMessage:    "user not found",
		},
		{
			name:               "should handle generic error correctly",
			err:                errors.New("generic error"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedMessage:    "Failed to validate user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			mockSessionValidator := mocks.NewMockSessionValidatorUseCase(t)

			middleware := NewAuthMiddleware(mockLogger, mockSessionValidator)

			rec := httptest.NewRecorder()
			middleware.handleValidationError(rec, tt.err)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)

			var response map[string]string
			err := json.NewDecoder(rec.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedMessage, response["message"])
		})
	}
}
