package tenant

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type TenantRepository struct {
	db  *sql.DB
	log ports.Logger
}

func (t *TenantRepository) FindByID(tenantID string) (*entities.Tenant, error) {

	query := `SELECT id, "createdAt", "name", description, "plan", "emailContact", logo, image, favicon, 
		"bgLogin", "customMenu", "externalCodes", subdomain, "customDomain", "mainColor", 
		"dropboxAppId", "dropboxMemberId", "dropboxRefreshToken", "dropboxAccessToken", 
		"dropboxAccessTokenValid", "import", "isOpenArea", "listFiles", "comments", 
		"hideCards", "hideYoutube", "bunnyLibraryApiKey", "bunnyLibraryId", token_api_auth, 
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = ?`

	var tenant entities.Tenant
	err := t.db.QueryRow(query, tenantID).Scan(
		&tenant.ID,
		&tenant.CreatedAt,
		&tenant.Name,
		&tenant.Description,
		&tenant.Plan,
		&tenant.EmailContact,
		&tenant.Logo,
		&tenant.Image,
		&tenant.Favicon,
		&tenant.BgLogin,
		&tenant.CustomMenu,
		&tenant.ExternalCodes,
		&tenant.SubDomain,
		&tenant.CustomDomain,
		&tenant.MainColor,
		&tenant.DropboxAppID,
		&tenant.DropboxMemberID,
		&tenant.DropboxRefreshToken,
		&tenant.DropboxAccessToken,
		&tenant.DropboxAccessTokenValid,
		&tenant.Import,
		&tenant.IsOpenArea,
		&tenant.ListFiles,
		&tenant.Comments,
		&tenant.HideCards,
		&tenant.HideYoutube,
		&tenant.BunnyLibraryApiKey,
		&tenant.BunnyLibraryID,
		&tenant.TokenApiAuth,
		&tenant.Language,
		&tenant.WebhookAPI,
		&tenant.RegisterNewUser,
		&tenant.AIEnabled,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrTenantNotFound
		}
		t.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: fmt.Errorf("error finding user by email: %w", err).Error(),
		}
	}

	return &tenant, nil
}

func (t *TenantRepository) FindBunnyInfoByID(tenantID string) (*entities.Tenant, error) {
	query := `SELECT id, bunnyLibraryApiKey, bunnyLibraryId
				FROM "Tenant" WHERE id = ?`

	var tenant entities.Tenant
	err := t.db.QueryRow(query, tenantID).Scan(
		&tenant.ID,
		&tenant.BunnyLibraryApiKey,
		&tenant.BunnyLibraryID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrTenantNotFound
		}
		t.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: fmt.Errorf("error finding tenant by id: %w", err).Error(),
		}
	}

	return &tenant, nil
}

func NewTenantRepository(db *sql.DB, log ports.Logger) *TenantRepository {
	return &TenantRepository{
		db:  db,
		log: log,
	}
}
