package tenant

import (
	"context"
	"database/sql"
	"errors"

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
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = $1`

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
			Message: "error finding tenant",
		}
	}

	return &tenant, nil
}

func (t *TenantRepository) FindBunnyInfoByID(tenantID string) (*entities.Tenant, error) {
	query := `SELECT id, "bunnyLibraryApiKey", "bunnyLibraryId"
				FROM "Tenant" WHERE id = $1`

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
			Message: "error finding tenant"}
	}

	return &tenant, nil
}

func (t *TenantRepository) FindTenantByToken(ctx context.Context, token string) (*entities.Tenant, error) {

	//TODO: create index to token_api_auth
	query := `
  SELECT id, name 
  FROM "Tenant" 
  WHERE token_api_auth = $1 
  LIMIT 1
`

	var tenant entities.Tenant
	err := t.db.QueryRowContext(ctx, query, token).Scan(
		&tenant.ID,
		&tenant.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrTenantNotFound
		}
		t.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding tenant with token",
		}
	}
	return &tenant, nil
}

func (t *TenantRepository) UpdateTokenApiAuth(ctx context.Context, tenantID, tokenHash string) error {

	if tenantID == "" || tokenHash == "" {
		return errors.New("error: tenantID or tokenHash is empty")
	}

	query := `UPDATE "Tenant" SET token_api_auth = $1 WHERE id = $2`

	_, err := t.db.ExecContext(ctx, query, tokenHash, tenantID)
	if err != nil {
		t.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating token api auth",
		}
	}

	return nil

}

func NewTenantRepository(db *sql.DB, log ports.Logger) ports.TenantRepository {
	return &TenantRepository{
		db:  db,
		log: log,
	}
}
