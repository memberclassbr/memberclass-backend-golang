package user

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserRepository(t *testing.T) {
	t.Run("should create new user repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}

func TestUserRepository_FindByID(t *testing.T) {
	tests := []struct {
		name         string
		userID       string
		mockSetup    func(sqlmock.Sqlmock)
		expectError  bool
		expectedErr  error
		expectedUser *entities.User
	}{
		{
			name:   "should return user when found",
			userID: "user-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				username := "testuser"
				phone := "123456789"
				image := "https://example.com/image.jpg"
				referrals := 5
				emailVerified := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "username", "phone", "email", "emailVerified",
					"image", "createdAt", "updatedAt", "referrals",
				}).AddRow(
					"user-123", username, phone, "test@example.com", emailVerified,
					image, time.Now(), time.Now(), referrals,
				)
				sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
					WithArgs("user-123").WillReturnRows(rows)
			},
			expectError: false,
			expectedUser: &entities.User{
				ID:    "user-123",
				Email: "test@example.com",
			},
		},
		{
			name:   "should return ErrUserNotFound when user does not exist",
			userID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
					WithArgs("non-existent").WillReturnError(sql.ErrNoRows)
			},
			expectError:  true,
			expectedErr:  memberclasserrors.ErrUserNotFound,
			expectedUser: nil,
		},
		{
			name:   "should return MemberClassError when database error occurs",
			userID: "user-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
					WithArgs("user-123").WillReturnError(errors.New("database connection error"))
			},
			expectError: true,
			expectedErr: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding user",
			},
			expectedUser: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError && !errors.Is(tt.expectedErr, memberclasserrors.ErrUserNotFound) {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByID(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedUser.ID, result.ID)
				assert.Equal(t, tt.expectedUser.Email, result.Email)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestUserRepository_ExistsByID(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		mockSetup      func(sqlmock.Sqlmock)
		expectError    bool
		expectedErr    error
		expectedExists bool
	}{
		{
			name:   "should return true when user exists",
			userID: "user-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				sqlMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM "User" WHERE id = \$1\)`).
					WithArgs("user-123").WillReturnRows(rows)
			},
			expectError:    false,
			expectedExists: true,
		},
		{
			name:   "should return false when user does not exist",
			userID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM "User" WHERE id = \$1\)`).
					WithArgs("non-existent").WillReturnRows(rows)
			},
			expectError:    false,
			expectedExists: false,
		},
		{
			name:   "should return MemberClassError when database error occurs",
			userID: "user-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM "User" WHERE id = \$1\)`).
					WithArgs("user-123").WillReturnError(errors.New("database connection error"))
			},
			expectError: true,
			expectedErr: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error checking user existence",
			},
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.ExistsByID(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedExists, result)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestUserRepository_BelongsToTenant(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		tenantID        string
		mockSetup       func(sqlmock.Sqlmock)
		expectError     bool
		expectedErr     error
		expectedBelongs bool
	}{
		{
			name:     "should return true when user belongs to tenant",
			userID:   "user-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("user-123", "tenant-123").WillReturnRows(rows)
			},
			expectError:     false,
			expectedBelongs: true,
		},
		{
			name:     "should return false when user does not belong to tenant",
			userID:   "user-123",
			tenantID: "tenant-456",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("user-123", "tenant-456").WillReturnRows(rows)
			},
			expectError:     false,
			expectedBelongs: false,
		},
		{
			name:     "should return MemberClassError when database error occurs",
			userID:   "user-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("user-123", "tenant-123").WillReturnError(errors.New("database connection error"))
			},
			expectError: true,
			expectedErr: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error checking user tenant membership",
			},
			expectedBelongs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.BelongsToTenant(tt.userID, tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBelongs, result)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestUserRepository_FindByID_WithNullableFields(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		mockSetup func(sqlmock.Sqlmock)
	}{
		{
			name:   "should handle null name field",
			userID: "user-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "username", "phone", "email", "emailVerified",
					"image", "createdAt", "updatedAt", "referrals",
				}).AddRow(
					"user-123", nil, nil, "test@example.com", nil,
					nil, time.Now(), time.Now(), nil,
				)
				sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
					WithArgs("user-123").WillReturnRows(rows)
			},
		},
		{
			name:   "should handle all nullable fields as null",
			userID: "user-456",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "username", "phone", "email", "emailVerified",
					"image", "createdAt", "updatedAt", "referrals",
				}).AddRow(
					"user-456", nil, nil, "another@example.com", nil,
					nil, time.Now(), time.Now(), nil,
				)
				sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
					WithArgs("user-456").WillReturnRows(rows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByID(tt.userID)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.userID, result.ID)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestUserRepository_FindByID_WithAllFields(t *testing.T) {
	t.Run("should return user with all fields populated", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserRepository(db, mockLogger)

		username := "johndoe"
		phone := "+1234567890"
		image := "https://example.com/avatar.jpg"
		referrals := 10
		emailVerified := time.Now()
		createdAt := time.Now().Add(-24 * time.Hour)
		updatedAt := time.Now()

		rows := sqlmock.NewRows([]string{
			"id", "username", "phone", "email", "emailVerified",
			"image", "createdAt", "updatedAt", "referrals",
		}).AddRow(
			"user-full", username, phone, "john@example.com", emailVerified,
			image, createdAt, updatedAt, referrals,
		)
		sqlMock.ExpectQuery(`SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = \$1`).
			WithArgs("user-full").WillReturnRows(rows)

		result, err := repository.FindByID("user-full")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "user-full", result.ID)
		assert.Equal(t, "john@example.com", result.Email)
		assert.NotNil(t, result.Username)
		assert.Equal(t, username, *result.Username)
		assert.NotNil(t, result.Phone)
		assert.Equal(t, phone, *result.Phone)
		assert.NotNil(t, result.Image)
		assert.Equal(t, image, *result.Image)
		assert.NotNil(t, result.Referrals)
		assert.Equal(t, referrals, *result.Referrals)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})
}

func TestUserRepository_ExistsByID_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		mockSetup      func(sqlmock.Sqlmock)
		expectedExists bool
	}{
		{
			name:   "should handle empty user ID",
			userID: "",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM "User" WHERE id = \$1\)`).
					WithArgs("").WillReturnRows(rows)
			},
			expectedExists: false,
		},
		{
			name:   "should handle user ID with special characters",
			userID: "user-123-abc_xyz",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				sqlMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM "User" WHERE id = \$1\)`).
					WithArgs("user-123-abc_xyz").WillReturnRows(rows)
			},
			expectedExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.ExistsByID(tt.userID)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedExists, result)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestUserRepository_BelongsToTenant_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		tenantID        string
		mockSetup       func(sqlmock.Sqlmock)
		expectedBelongs bool
	}{
		{
			name:     "should handle empty user ID",
			userID:   "",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("", "tenant-123").WillReturnRows(rows)
			},
			expectedBelongs: false,
		},
		{
			name:     "should handle empty tenant ID",
			userID:   "user-123",
			tenantID: "",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("user-123", "").WillReturnRows(rows)
			},
			expectedBelongs: false,
		},
		{
			name:     "should handle both empty IDs",
			userID:   "",
			tenantID: "",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				sqlMock.ExpectQuery(`SELECT EXISTS\(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = \$1 AND "tenantId" = \$2
	\)`).
					WithArgs("", "").WillReturnRows(rows)
			},
			expectedBelongs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.BelongsToTenant(tt.userID, tt.tenantID)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBelongs, result)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

