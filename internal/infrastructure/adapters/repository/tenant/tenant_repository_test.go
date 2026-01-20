package tenant

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func stringPtr(s string) *string {
	return &s
}

func TestTenantRepository_FindByID(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		mockSetup      func(sqlmock.Sqlmock)
		expectedError  error
		expectedTenant *tenant.Tenant
	}{
		{
			name:     "should return tenant when found",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "createdAt", "name", "description", "plan", "emailContact",
					"logo", "image", "favicon", "bgLogin", "customMenu", "externalCodes",
					"subdomain", "customDomain", "mainColor", "dropboxAppId", "dropboxMemberId",
					"dropboxRefreshToken", "dropboxAccessToken", "dropboxAccessTokenValid",
					"import", "isOpenArea", "listFiles", "comments", "hideCards", "hideYoutube",
					"bunnyLibraryApiKey", "bunnyLibraryId", "token_api_auth", "language",
					"webhook_api", "registerNewUser", "aiEnabled",
				}).AddRow(
					"tenant-123", time.Now(), "Test Tenant", "Description", "Pro", "contact@test.com",
					"", "", "", "", "", "", "", "", "", "", "", "", "", time.Now(),
					false, false, false, "", false, false, "bunny-api-key", "bunny-library-id",
					"", "", "", false, false,
				)
				sqlMock.ExpectQuery(`SELECT id, "createdAt", "name", description, "plan", "emailContact", logo, image, favicon, 
		"bgLogin", "customMenu", "externalCodes", subdomain, "customDomain", "mainColor", 
		"dropboxAppId", "dropboxMemberId", "dropboxRefreshToken", "dropboxAccessToken", 
		"dropboxAccessTokenValid", "import", "isOpenArea", "listFiles", "comments", 
		"hideCards", "hideYoutube", "bunnyLibraryApiKey", "bunnyLibraryId", token_api_auth, 
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = \$1`).
					WithArgs("tenant-123").WillReturnRows(rows)
			},
			expectedError: nil,
			expectedTenant: &tenant.Tenant{
				ID:                 "tenant-123",
				Name:               "Test Tenant",
				Description:        stringPtr("Description"),
				Plan:               stringPtr("Pro"),
				EmailContact:       stringPtr("contact@test.com"),
				BunnyLibraryApiKey: stringPtr("bunny-api-key"),
				BunnyLibraryID:     stringPtr("bunny-library-id"),
			},
		},
		{
			name:     "should return ErrTenantNotFound when tenant does not exist",
			tenantID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "createdAt", "name", description, "plan", "emailContact", logo, image, favicon, 
		"bgLogin", "customMenu", "externalCodes", subdomain, "customDomain", "mainColor", 
		"dropboxAppId", "dropboxMemberId", "dropboxRefreshToken", "dropboxAccessToken", 
		"dropboxAccessTokenValid", "import", "isOpenArea", "listFiles", "comments", 
		"hideCards", "hideYoutube", "bunnyLibraryApiKey", "bunnyLibraryId", token_api_auth, 
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = \$1`).
					WithArgs("non-existent").WillReturnError(sql.ErrNoRows)
			},
			expectedError:  memberclasserrors.ErrTenantNotFound,
			expectedTenant: nil,
		},
		{
			name:     "should return MemberClassError when database error occurs",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "createdAt", "name", description, "plan", "emailContact", logo, image, favicon, 
		"bgLogin", "customMenu", "externalCodes", subdomain, "customDomain", "mainColor", 
		"dropboxAppId", "dropboxMemberId", "dropboxRefreshToken", "dropboxAccessToken", 
		"dropboxAccessTokenValid", "import", "isOpenArea", "listFiles", "comments", 
		"hideCards", "hideYoutube", "bunnyLibraryApiKey", "bunnyLibraryId", token_api_auth, 
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = \$1`).
					WithArgs("tenant-123").WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding tenant",
			},
			expectedTenant: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil && !errors.Is(tt.expectedError, memberclasserrors.ErrTenantNotFound) {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewTenantRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByID(tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedTenant.ID, result.ID)
				assert.Equal(t, tt.expectedTenant.Name, result.Name)
				if tt.expectedTenant.Description != nil {
					assert.Equal(t, tt.expectedTenant.Description, result.Description)
				}
				if tt.expectedTenant.Plan != nil {
					assert.Equal(t, tt.expectedTenant.Plan, result.Plan)
				}
				if tt.expectedTenant.EmailContact != nil {
					assert.Equal(t, tt.expectedTenant.EmailContact, result.EmailContact)
				}
				if tt.expectedTenant.BunnyLibraryApiKey != nil {
					assert.Equal(t, tt.expectedTenant.BunnyLibraryApiKey, result.BunnyLibraryApiKey)
				}
				if tt.expectedTenant.BunnyLibraryID != nil {
					assert.Equal(t, tt.expectedTenant.BunnyLibraryID, result.BunnyLibraryID)
				}
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestTenantRepository_FindBunnyInfoByID(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		mockSetup      func(sqlmock.Sqlmock)
		expectedError  error
		expectedTenant *tenant.Tenant
	}{
		{
			name:     "should return tenant bunny info when found",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "bunnyLibraryApiKey", "bunnyLibraryId"}).
					AddRow("tenant-123", "bunny-api-key", "bunny-library-id")
				sqlMock.ExpectQuery(`SELECT id, "bunnyLibraryApiKey", "bunnyLibraryId"
				FROM "Tenant" WHERE id = \$1`).
					WithArgs("tenant-123").WillReturnRows(rows)
			},
			expectedError: nil,
			expectedTenant: &tenant.Tenant{
				ID:                 "tenant-123",
				BunnyLibraryApiKey: stringPtr("bunny-api-key"),
				BunnyLibraryID:     stringPtr("bunny-library-id"),
			},
		},
		{
			name:     "should return ErrTenantNotFound when tenant does not exist",
			tenantID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "bunnyLibraryApiKey", "bunnyLibraryId"
				FROM "Tenant" WHERE id = \$1`).
					WithArgs("non-existent").WillReturnError(sql.ErrNoRows)
			},
			expectedError:  memberclasserrors.ErrTenantNotFound,
			expectedTenant: nil,
		},
		{
			name:     "should return MemberClassError when database error occurs",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "bunnyLibraryApiKey", "bunnyLibraryId"
				FROM "Tenant" WHERE id = \$1`).
					WithArgs("tenant-123").WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding tenant",
			},
			expectedTenant: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil && !errors.Is(tt.expectedError, memberclasserrors.ErrTenantNotFound) {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewTenantRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindBunnyInfoByID(tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedTenant.ID, result.ID)
				if tt.expectedTenant.BunnyLibraryApiKey != nil {
					assert.Equal(t, tt.expectedTenant.BunnyLibraryApiKey, result.BunnyLibraryApiKey)
				}
				if tt.expectedTenant.BunnyLibraryID != nil {
					assert.Equal(t, tt.expectedTenant.BunnyLibraryID, result.BunnyLibraryID)
				}
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestTenantRepository_FindTenantByToken(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		mockSetup      func(sqlmock.Sqlmock)
		expectedError  error
		expectedTenant *tenant.Tenant
	}{
		{
			name:  "should return tenant when found by token",
			token: "token-hash-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow("tenant-123", "Test Tenant")
				sqlMock.ExpectQuery(`SELECT id, name`).
					WithArgs("token-hash-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedTenant: &tenant.Tenant{
				ID:   "tenant-123",
				Name: "Test Tenant",
			},
		},
		{
			name:  "should return ErrTenantNotFound when tenant does not exist",
			token: "non-existent-token",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, name`).
					WithArgs("non-existent-token").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError:  memberclasserrors.ErrTenantNotFound,
			expectedTenant: nil,
		},
		{
			name:  "should return MemberClassError when database error occurs",
			token: "token-hash-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, name`).
					WithArgs("token-hash-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding tenant with token",
			},
			expectedTenant: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil && !errors.Is(tt.expectedError, memberclasserrors.ErrTenantNotFound) {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewTenantRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindTenantByToken(context.Background(), tt.token)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedError, memberclasserrors.ErrTenantNotFound) {
					assert.Equal(t, memberclasserrors.ErrTenantNotFound, err)
				} else {
					assert.Equal(t, tt.expectedError, err)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedTenant.ID, result.ID)
				assert.Equal(t, tt.expectedTenant.Name, result.Name)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestTenantRepository_UpdateTokenApiAuth(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		tokenHash     string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
	}{
		{
			name:      "should update token successfully",
			tenantID:  "tenant-123",
			tokenHash: "token-hash-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectExec(`UPDATE "Tenant" SET token_api_auth`).
					WithArgs("token-hash-123", "tenant-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectedError: nil,
		},
		{
			name:      "should return error when tenantID is empty",
			tenantID:  "",
			tokenHash: "token-hash-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
			},
			expectedError: errors.New("error: tenantID or tokenHash is empty"),
		},
		{
			name:      "should return error when tokenHash is empty",
			tenantID:  "tenant-123",
			tokenHash: "",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
			},
			expectedError: errors.New("error: tenantID or tokenHash is empty"),
		},
		{
			name:      "should return error when both are empty",
			tenantID:  "",
			tokenHash: "",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
			},
			expectedError: errors.New("error: tenantID or tokenHash is empty"),
		},
		{
			name:      "should return MemberClassError when database error occurs",
			tenantID:  "tenant-123",
			tokenHash: "token-hash-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectExec(`UPDATE "Tenant" SET token_api_auth`).
					WithArgs("token-hash-123", "tenant-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error updating token api auth",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				validationErr := errors.New("error: tenantID or tokenHash is empty")
				if tt.expectedError.Error() == validationErr.Error() {
				} else {
					mockLogger.EXPECT().Error(mock.Anything).Return()
				}
			}

			repository := NewTenantRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			err = repository.UpdateTokenApiAuth(context.Background(), tt.tenantID, tt.tokenHash)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedError, errors.New("error: tenantID or tokenHash is empty")) {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				} else {
					assert.Equal(t, tt.expectedError, err)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestNewTenantRepository(t *testing.T) {
	t.Run("should create new tenant repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewTenantRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}
