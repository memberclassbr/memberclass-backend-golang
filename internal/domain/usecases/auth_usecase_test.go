package usecases

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAuthUseCase(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockTenantRepo := mocks.NewMockTenantRepository(t)
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewAuthUseCase(mockUserRepo, mockTenantRepo, mockCache, mockLogger)

	assert.NotNil(t, useCase)
}

func TestAuthUseCase_GenerateMagicLink(t *testing.T) {
	os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
	defer os.Unsetenv("PUBLIC_ROOT_DOMAIN")

	customDomain := "custom.example.com"
	subdomain := "mysubdomain"
	emptyString := ""

	tests := []struct {
		name          string
		req           request.AuthRequest
		tenantID      string
		mockSetup     func(*mocks.MockUserRepository, *mocks.MockTenantRepository, *mocks.MockCache, *mocks.MockLogger)
		expectError   bool
		expectedError *memberclasserrors.MemberClassError
		validateResult func(*testing.T, interface{})
	}{
		{
			name:     "should return error when email is empty",
			req:      request.AuthRequest{Email: ""},
			tenantID: "tenant-123",
			mockSetup: func(*mocks.MockUserRepository, *mocks.MockTenantRepository, *mocks.MockCache, *mocks.MockLogger) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "Email é obrigatório e deve ser uma string",
			},
		},
		{
			name:     "should return cached link when cache hit",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				cachedLink := "https://custom.example.com/login?token=abc123&email=user@example.com&isReset=false"
				mockCache.EXPECT().Get(mock.Anything, "auth_cache:tenant-123:user@example.com").Return(cachedLink, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Equal(t, "https://custom.example.com/login?token=abc123&email=user@example.com&isReset=false", authResp.Link)
			},
		},
		{
			name:     "should return error when user not found",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(nil, memberclasserrors.ErrUserNotFound)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Usuário não encontrado",
			},
		},
		{
			name:     "should return error when user repository returns generic error",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "should return error when user not in tenant",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(false, nil)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Usuário não encontrado no tenant",
			},
		},
		{
			name:     "should return error when BelongsToTenant returns error",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(false, errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error checking user tenant membership",
			},
		},
		{
			name:     "should return error when tenant not found in buildLoginLink",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(nil, memberclasserrors.ErrTenantNotFound)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error building login link",
			},
		},
		{
			name:     "should return error when UpdateMagicToken fails",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "should return error when PUBLIC_ROOT_DOMAIN is not set",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Unsetenv("PUBLIC_ROOT_DOMAIN")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error building login link",
			},
		},
		{
			name:     "should return success with customDomain",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:           "tenant-123",
					CustomDomain: &customDomain,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "custom.example.com")
				assert.Contains(t, authResp.Link, "token=")
				assert.Contains(t, authResp.Link, "email=user@example.com")
				assert.Contains(t, authResp.Link, "isReset=false")
			},
		},
		{
			name:     "should return success with subdomain",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: &subdomain,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "mysubdomain.example.com")
			},
		},
		{
			name:     "should return success with default subdomain when SubDomain is empty",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: &emptyString,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "acessos.example.com")
			},
		},
		{
			name:     "should return success with default subdomain when SubDomain is nil",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: nil,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "acessos.example.com")
			},
		},
		{
			name:     "should use http protocol for localhost",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "localhost:8181")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: &subdomain,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "http://")
			},
		},
		{
			name:     "should use https protocol for production domain",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: &subdomain,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.Contains(t, authResp.Link, "https://")
			},
		},
		{
			name:     "should continue even when cache Set fails",
			req:      request.AuthRequest{Email: "user@example.com"},
			tenantID: "tenant-123",
			mockSetup: func(mockUserRepo *mocks.MockUserRepository, mockTenantRepo *mocks.MockTenantRepository, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				os.Setenv("PUBLIC_ROOT_DOMAIN", "example.com")
				mockCache.EXPECT().Get(mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("redis: nil"))
				mockUserRepo.EXPECT().FindByEmail("user@example.com").Return(&entities.User{ID: "user-123"}, nil)
				mockUserRepo.EXPECT().BelongsToTenant("user-123", "tenant-123").Return(true, nil)
				mockUserRepo.EXPECT().UpdateMagicToken(mock.Anything, "user-123", mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&entities.Tenant{
					ID:        "tenant-123",
					SubDomain: &subdomain,
				}, nil)
				mockCache.EXPECT().Set(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), 300*time.Second).Return(errors.New("cache error"))
				mockLogger.EXPECT().Error("Error caching auth response: cache error")
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				authResp := result.(*response.AuthResponse)
				assert.True(t, authResp.OK)
				assert.NotEmpty(t, authResp.Link)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserRepo := mocks.NewMockUserRepository(t)
			mockTenantRepo := mocks.NewMockTenantRepository(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUserRepo, mockTenantRepo, mockCache, mockLogger)

			useCase := NewAuthUseCase(mockUserRepo, mockTenantRepo, mockCache, mockLogger)

			result, err := useCase.GenerateMagicLink(context.Background(), tt.req, tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						if tt.expectedError.Message != "" {
							assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
						}
					}
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.True(t, result.OK)
				assert.NotEmpty(t, result.Link)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}
