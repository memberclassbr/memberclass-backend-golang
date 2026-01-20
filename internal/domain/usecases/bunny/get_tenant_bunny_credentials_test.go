package bunny

import (
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetTenantBunnyCredentialsUseCase_Execute_Success(t *testing.T) {
	mockRepo := mocks.NewMockTenantRepository(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewTenantGetTenantBunnyCredentialsUseCase(mockRepo, mockLogger)

	tenantID := "test-tenant"
	bunnyLibraryID := "test-library"
	bunnyLibraryApiKey := "test-key"
	expectedTenant := &tenant.Tenant{
		ID:                 tenantID,
		BunnyLibraryID:     &bunnyLibraryID,
		BunnyLibraryApiKey: &bunnyLibraryApiKey,
	}
	expectedCredentials := &dto.TenantBunnyCredentials{
		TenantID:           tenantID,
		BunnyLibraryID:     "test-library",
		BunnyLibraryApiKey: "test-key",
	}

	mockRepo.EXPECT().
		FindBunnyInfoByID(tenantID).
		Return(expectedTenant, nil).
		Once()

	mockLogger.EXPECT().
		Info(mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	result, err := useCase.Execute(tenantID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCredentials, result)
}

func TestGetTenantBunnyCredentialsUseCase_Execute_TenantNotFound(t *testing.T) {
	mockRepo := mocks.NewMockTenantRepository(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewTenantGetTenantBunnyCredentialsUseCase(mockRepo, mockLogger)

	tenantID := "invalid-tenant"

	mockRepo.EXPECT().
		FindBunnyInfoByID(tenantID).
		Return(nil, memberclasserrors.ErrTenantNotFound).
		Once()

	mockLogger.EXPECT().
		Error(mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	result, err := useCase.Execute(tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrTenantNotFound, err)
}

func TestGetTenantBunnyCredentialsUseCase_Execute_EmptyTenantID(t *testing.T) {
	mockRepo := mocks.NewMockTenantRepository(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewTenantGetTenantBunnyCredentialsUseCase(mockRepo, mockLogger)

	tenantID := ""

	mockLogger.EXPECT().
		Error(mock.Anything, mock.Anything).
		Maybe()

	result, err := useCase.Execute(tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrTenantIDEmpty, err)
}
