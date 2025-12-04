package usecases

import (
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewValidateSessionUseCase(t *testing.T) {
	t.Run("should create new validate session use case instance", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockLogger := mocks.NewMockLogger(t)

		useCase := NewValidateSessionUseCase(mockUserRepo, mockLogger)

		assert.NotNil(t, useCase)
	})
}

func TestValidateSessionUseCase_ValidateUserExists(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		setupMocks  func(*mocks.MockUserRepository, *mocks.MockLogger)
		expectError bool
		expectedErr error
	}{
		{
			name:   "should return nil when user exists",
			userID: "user-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("ExistsByID", "user-123").Return(true, nil)
			},
			expectError: false,
		},
		{
			name:   "should return ErrUserNotFound when user does not exist",
			userID: "user-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("ExistsByID", "user-123").Return(false, nil)
				logger.On("Debug", mock.Anything, mock.Anything).Maybe()
			},
			expectError: true,
			expectedErr: memberclasserrors.ErrUserNotFound,
		},
		{
			name:   "should return ErrUserNotFound when userID is empty",
			userID: "",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
			},
			expectError: true,
			expectedErr: memberclasserrors.ErrUserNotFound,
		},
		{
			name:   "should return error when repository returns error",
			userID: "user-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("ExistsByID", "user-123").Return(false, errors.New("database error"))
				logger.On("Error", mock.Anything, mock.Anything).Maybe()
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserRepo := mocks.NewMockUserRepository(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.setupMocks(mockUserRepo, mockLogger)

			useCase := NewValidateSessionUseCase(mockUserRepo, mockLogger)

			err := useCase.ValidateUserExists(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSessionUseCase_ValidateUserBelongsToTenant(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		tenantID    string
		setupMocks  func(*mocks.MockUserRepository, *mocks.MockLogger)
		expectError bool
		expectedErr error
	}{
		{
			name:     "should return nil when user belongs to tenant",
			userID:   "user-123",
			tenantID: "tenant-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
			},
			expectError: false,
		},
		{
			name:     "should return ErrUserNotInTenant when user does not belong to tenant",
			userID:   "user-123",
			tenantID: "tenant-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(false, nil)
				logger.On("Debug", mock.Anything, mock.Anything).Maybe()
			},
			expectError: true,
			expectedErr: memberclasserrors.ErrUserNotInTenant,
		},
		{
			name:     "should return ErrUserNotInTenant when userID is empty",
			userID:   "",
			tenantID: "tenant-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
			},
			expectError: true,
			expectedErr: memberclasserrors.ErrUserNotInTenant,
		},
		{
			name:     "should return ErrUserNotInTenant when tenantID is empty",
			userID:   "user-123",
			tenantID: "",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
			},
			expectError: true,
			expectedErr: memberclasserrors.ErrUserNotInTenant,
		},
		{
			name:     "should return error when repository returns error",
			userID:   "user-123",
			tenantID: "tenant-123",
			setupMocks: func(userRepo *mocks.MockUserRepository, logger *mocks.MockLogger) {
				userRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(false, errors.New("database error"))
				logger.On("Error", mock.Anything, mock.Anything).Maybe()
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserRepo := mocks.NewMockUserRepository(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.setupMocks(mockUserRepo, mockLogger)

			useCase := NewValidateSessionUseCase(mockUserRepo, mockLogger)

			err := useCase.ValidateUserBelongsToTenant(tt.userID, tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

