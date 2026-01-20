package sso

import (
	"context"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/sso"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGenerateSSOToken_Success(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	req := sso.GenerateSSOTokenRequest{
		UserID:   "user123",
		TenantID: "tenant456",
	}
	externalURL := "https://external.com"

	mockUserRepo.On("ExistsByID", req.UserID).Return(true, nil)
	mockUserRepo.On("BelongsToTenant", req.UserID, req.TenantID).Return(true, nil)
	mockSSORepo.On("UpdateSSOToken", ctx, req.UserID, req.TenantID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)

	result, err := useCase.GenerateSSOToken(ctx, req, externalURL)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Token)
	assert.Contains(t, result.RedirectURL, "token-mc=")
	assert.Equal(t, 300, result.ExpiresInSecs)

	mockUserRepo.AssertExpectations(t)
	mockSSORepo.AssertExpectations(t)
}

func TestGenerateSSOToken_UserNotFound(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	req := sso.GenerateSSOTokenRequest{
		UserID:   "nonexistent",
		TenantID: "tenant456",
	}

	mockUserRepo.On("ExistsByID", req.UserID).Return(false, nil)

	result, err := useCase.GenerateSSOToken(ctx, req, "https://external.com")

	assert.Error(t, err)
	assert.Nil(t, result)

	memberClassErr, ok := err.(*memberclasserrors.MemberClassError)
	assert.True(t, ok)
	assert.Equal(t, 404, memberClassErr.Code)

	mockUserRepo.AssertExpectations(t)
}

func TestGenerateSSOToken_UserNotInTenant(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	req := sso.GenerateSSOTokenRequest{
		UserID:   "user123",
		TenantID: "tenant456",
	}

	mockUserRepo.On("ExistsByID", req.UserID).Return(true, nil)
	mockUserRepo.On("BelongsToTenant", req.UserID, req.TenantID).Return(false, nil)

	result, err := useCase.GenerateSSOToken(ctx, req, "https://external.com")

	assert.Error(t, err)
	assert.Nil(t, result)

	memberClassErr, ok := err.(*memberclasserrors.MemberClassError)
	assert.True(t, ok)
	assert.Equal(t, 403, memberClassErr.Code)

	mockUserRepo.AssertExpectations(t)
}

func TestValidateSSOToken_Success(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	token := "valid-token"
	ip := "192.168.1.1"

	userName := "Test User"
	userPhone := "+5511999999999"
	userDoc := "12345678900"

	expectedResponse := &response.ValidateSSOTokenResponse{
		User: response.SSOUserData{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     &userName,
			Phone:    &userPhone,
			Document: &userDoc,
		},
		Tenant: response.SSOTenantData{
			ID:   "tenant456",
			Name: "Test Tenant",
		},
	}

	mockSSORepo.On("ValidateAndConsumeSSOToken", ctx, mock.AnythingOfType("string"), ip).Return(expectedResponse, nil)

	result, err := useCase.ValidateSSOToken(ctx, token, ip)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "user123", result.User.ID)
	assert.Equal(t, "test@example.com", result.User.Email)
	assert.Equal(t, "tenant456", result.Tenant.ID)

	mockSSORepo.AssertExpectations(t)
}

func TestValidateSSOToken_EmptyToken(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	result, err := useCase.ValidateSSOToken(ctx, "", "192.168.1.1")

	assert.Error(t, err)
	assert.Nil(t, result)

	memberClassErr, ok := err.(*memberclasserrors.MemberClassError)
	assert.True(t, ok)
	assert.Equal(t, 400, memberClassErr.Code)
}

func TestValidateSSOToken_InvalidToken(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := NewSSOUseCase(mockSSORepo, mockUserRepo, mockLogger)

	ctx := context.Background()
	token := "invalid-token"
	ip := "192.168.1.1"

	mockSSORepo.On("ValidateAndConsumeSSOToken", ctx, mock.AnythingOfType("string"), ip).Return(nil, &memberclasserrors.MemberClassError{
		Code:    401,
		Message: "token inválido",
	})

	result, err := useCase.ValidateSSOToken(ctx, token, ip)

	assert.Error(t, err)
	assert.Nil(t, result)

	mockSSORepo.AssertExpectations(t)
}

func TestBuildRedirectURL(t *testing.T) {
	mockSSORepo := new(mocks.MockSSORepository)
	mockUserRepo := new(mocks.MockUserRepository)
	mockLogger := new(mocks.MockLogger)

	useCase := &SSOUseCase{
		ssoRepo:  mockSSORepo,
		userRepo: mockUserRepo,
		logger:   mockLogger,
	}

	tests := []struct {
		name        string
		externalURL string
		token       string
		expected    string
		expectError bool
	}{
		{
			name:        "URL simples",
			externalURL: "https://external.com",
			token:       "test-token",
			expected:    "https://external.com?token-mc=test-token",
			expectError: false,
		},
		{
			name:        "URL com query params existentes",
			externalURL: "https://external.com?param=value",
			token:       "test-token",
			expected:    "https://external.com?param=value&token-mc=test-token",
			expectError: false,
		},
		{
			name:        "URL inválida",
			externalURL: "://invalid-url",
			token:       "test-token",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := useCase.buildRedirectURL(tt.externalURL, tt.token)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, result, "token-mc="+tt.token)
			}
		})
	}
}
