package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAITenantUseCase(t *testing.T) {
	mockTenantRepo := mocks.NewMockTenantRepository(t)
	mockAILessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	mockLogger.EXPECT().Warn("TRANSCRIPTION_API_URL not configured").Maybe()

	useCase := NewAITenantUseCase(mockTenantRepo, mockAILessonUseCase, mockLogger)

	assert.NotNil(t, useCase)
}

func TestAITenantUseCase_GetTenantsWithAIEnabled(t *testing.T) {
	bunnyLibraryID1 := "library-123"
	bunnyLibraryApiKey1 := "api-key-123"
	bunnyLibraryID2 := "library-456"
	bunnyLibraryApiKey2 := "api-key-456"

	tests := []struct {
		name           string
		mockSetup      func(*mocks.MockTenantRepository, *mocks.MockLogger)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *response.AITenantsResponse)
	}{
		{
			name: "should return error when FindAllWithAIEnabled fails",
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindAllWithAIEnabled(mock.Anything).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name: "should return empty list when no tenants found",
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindAllWithAIEnabled(mock.Anything).Return([]*entities.Tenant{}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AITenantsResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 0, result.Total)
				assert.Empty(t, result.Tenants)
			},
		},
		{
			name: "should return success with tenants that have bunny library info",
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindAllWithAIEnabled(mock.Anything).Return([]*entities.Tenant{
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
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AITenantsResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 2, result.Total)
				assert.Len(t, result.Tenants, 2)
				assert.Equal(t, "tenant-1", result.Tenants[0].ID)
				assert.Equal(t, "Tenant 1", result.Tenants[0].Name)
				assert.True(t, result.Tenants[0].AIEnabled)
				assert.NotNil(t, result.Tenants[0].BunnyLibraryID)
				assert.Equal(t, bunnyLibraryID1, *result.Tenants[0].BunnyLibraryID)
				assert.NotNil(t, result.Tenants[0].BunnyLibraryApiKey)
				assert.Equal(t, bunnyLibraryApiKey1, *result.Tenants[0].BunnyLibraryApiKey)
				assert.Equal(t, "tenant-2", result.Tenants[1].ID)
				assert.Equal(t, "Tenant 2", result.Tenants[1].Name)
				assert.True(t, result.Tenants[1].AIEnabled)
				assert.NotNil(t, result.Tenants[1].BunnyLibraryID)
				assert.Equal(t, bunnyLibraryID2, *result.Tenants[1].BunnyLibraryID)
				assert.NotNil(t, result.Tenants[1].BunnyLibraryApiKey)
				assert.Equal(t, bunnyLibraryApiKey2, *result.Tenants[1].BunnyLibraryApiKey)
			},
		},
		{
			name: "should return success with tenants that have null bunny library info",
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindAllWithAIEnabled(mock.Anything).Return([]*entities.Tenant{
					{
						ID:        "tenant-1",
						Name:      "Tenant 1",
						AIEnabled: true,
					},
					{
						ID:        "tenant-2",
						Name:      "Tenant 2",
						AIEnabled: true,
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AITenantsResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 2, result.Total)
				assert.Len(t, result.Tenants, 2)
				assert.Equal(t, "tenant-1", result.Tenants[0].ID)
				assert.Nil(t, result.Tenants[0].BunnyLibraryID)
				assert.Nil(t, result.Tenants[0].BunnyLibraryApiKey)
				assert.Equal(t, "tenant-2", result.Tenants[1].ID)
				assert.Nil(t, result.Tenants[1].BunnyLibraryID)
				assert.Nil(t, result.Tenants[1].BunnyLibraryApiKey)
			},
		},
		{
			name: "should return success with mixed tenants",
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindAllWithAIEnabled(mock.Anything).Return([]*entities.Tenant{
					{
						ID:                 "tenant-1",
						Name:               "Tenant 1",
						AIEnabled:          true,
						BunnyLibraryID:     &bunnyLibraryID1,
						BunnyLibraryApiKey: &bunnyLibraryApiKey1,
					},
					{
						ID:        "tenant-2",
						Name:      "Tenant 2",
						AIEnabled: true,
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AITenantsResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 2, result.Total)
				assert.Len(t, result.Tenants, 2)
				assert.NotNil(t, result.Tenants[0].BunnyLibraryID)
				assert.NotNil(t, result.Tenants[0].BunnyLibraryApiKey)
				assert.Nil(t, result.Tenants[1].BunnyLibraryID)
				assert.Nil(t, result.Tenants[1].BunnyLibraryApiKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTenantRepo := mocks.NewMockTenantRepository(t)
			mockAILessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			mockLogger.EXPECT().Warn("TRANSCRIPTION_API_URL not configured").Maybe()

			tt.mockSetup(mockTenantRepo, mockLogger)

			useCase := NewAITenantUseCase(mockTenantRepo, mockAILessonUseCase, mockLogger)

			result, err := useCase.GetTenantsWithAIEnabled(context.Background())

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
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}
