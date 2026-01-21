package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewApiTokenTenantUseCase(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	assert.NotNil(t, useCase)
}

func TestApiTokenTenantUseCase_GenerateToken_Success(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
	}

	mockRepo.On("FindByID", tenantID).Return(tenant, nil)
	mockRepo.On("UpdateTokenApiAuth", mock.Anything, tenantID, mock.AnythingOfType("string")).Return(nil)

	token, err := useCase.GenerateToken(context.Background(), tenantID)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Len(t, token, 64)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_GenerateToken_EmptyTenantID(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	token, err := useCase.GenerateToken(context.Background(), "")

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Equal(t, "tenantID is required", err.Error())
}

func TestApiTokenTenantUseCase_GenerateToken_TenantNotFound(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	tenantID := "non-existent"
	repoError := memberclasserrors.ErrTenantNotFound

	mockRepo.On("FindByID", tenantID).Return(nil, repoError)

	token, err := useCase.GenerateToken(context.Background(), tenantID)

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_GenerateToken_RepositoryErrorOnFind(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	tenantID := "tenant-123"
	repoError := errors.New("database error")

	mockRepo.On("FindByID", tenantID).Return(nil, repoError)

	token, err := useCase.GenerateToken(context.Background(), tenantID)

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_GenerateToken_RepositoryErrorOnUpdate(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
	}

	updateError := errors.New("update error")

	mockRepo.On("FindByID", tenantID).Return(tenant, nil)
	mockRepo.On("UpdateTokenApiAuth", mock.Anything, tenantID, mock.AnythingOfType("string")).Return(updateError)

	token, err := useCase.GenerateToken(context.Background(), tenantID)

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Equal(t, updateError, err)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_ValidateToken_Success(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	token := "test-token-hash"
	tenantID := "tenant-123"
	tenant := &tenant.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
	}

	mockRepo.On("FindTenantByToken", mock.Anything, token).Return(tenant, nil)

	result, err := useCase.ValidateToken(context.Background(), token)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, tenantID, result.ID)
	assert.Equal(t, "Test Tenant", result.Name)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_ValidateToken_EmptyToken(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	result, err := useCase.ValidateToken(context.Background(), "")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "token is required", err.Error())
}

func TestApiTokenTenantUseCase_ValidateToken_TenantNotFound(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	token := "invalid-token"
	repoError := memberclasserrors.ErrTenantNotFound

	mockRepo.On("FindTenantByToken", mock.Anything, token).Return(nil, repoError)

	result, err := useCase.ValidateToken(context.Background(), token)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_ValidateToken_RepositoryError(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	token := "test-token"
	repoError := errors.New("database error")

	mockRepo.On("FindTenantByToken", mock.Anything, token).Return(nil, repoError)

	result, err := useCase.ValidateToken(context.Background(), token)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestApiTokenTenantUseCase_GenerateToken_UniqueTokens(t *testing.T) {
	mockRepo := new(mocks.MockTenantRepository)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewApiTokenTenantUseCase(mockRepo, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
	}

	mockRepo.On("FindByID", tenantID).Return(tenant, nil).Times(2)
	mockRepo.On("UpdateTokenApiAuth", mock.Anything, tenantID, mock.AnythingOfType("string")).Return(nil).Times(2)

	token1, err1 := useCase.GenerateToken(context.Background(), tenantID)
	token2, err2 := useCase.GenerateToken(context.Background(), tenantID)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2)

	mockRepo.AssertExpectations(t)
}
