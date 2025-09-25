package tenant

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

func TestTenantRepository_FindByID(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedTenant *entities.Tenant
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
			expectedTenant: &entities.Tenant{
				ID: "tenant-123",
				Name: "Test Tenant",
				Description: "Description",
				Plan: "Pro",
				EmailContact: "contact@test.com",
				BunnyLibraryApiKey: "bunny-api-key",
				BunnyLibraryID: "bunny-library-id",
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
			expectedError: memberclasserrors.ErrTenantNotFound,
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
				assert.Equal(t, tt.expectedTenant.Description, result.Description)
				assert.Equal(t, tt.expectedTenant.Plan, result.Plan)
				assert.Equal(t, tt.expectedTenant.EmailContact, result.EmailContact)
				assert.Equal(t, tt.expectedTenant.BunnyLibraryApiKey, result.BunnyLibraryApiKey)
				assert.Equal(t, tt.expectedTenant.BunnyLibraryID, result.BunnyLibraryID)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestTenantRepository_FindBunnyInfoByID(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedTenant *entities.Tenant
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
			expectedTenant: &entities.Tenant{
				ID: "tenant-123",
				BunnyLibraryApiKey: "bunny-api-key",
				BunnyLibraryID: "bunny-library-id",
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
			expectedError: memberclasserrors.ErrTenantNotFound,
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
				assert.Equal(t, tt.expectedTenant.BunnyLibraryApiKey, result.BunnyLibraryApiKey)
				assert.Equal(t, tt.expectedTenant.BunnyLibraryID, result.BunnyLibraryID)
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