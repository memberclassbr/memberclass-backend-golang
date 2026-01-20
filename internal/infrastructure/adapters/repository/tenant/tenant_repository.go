package tenant

import (
	"context"
	"database/sql"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	tenant2 "github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

type TenantRepository struct {
	db  *sql.DB
	log ports.Logger
}

func (t *TenantRepository) FindByID(tenantID string) (*tenant.Tenant, error) {

	query := `SELECT id, "createdAt", "name", description, "plan", "emailContact", logo, image, favicon, 
		"bgLogin", "customMenu", "externalCodes", subdomain, "customDomain", "mainColor", 
		"dropboxAppId", "dropboxMemberId", "dropboxRefreshToken", "dropboxAccessToken", 
		"dropboxAccessTokenValid", "import", "isOpenArea", "listFiles", "comments", 
		"hideCards", "hideYoutube", "bunnyLibraryApiKey", "bunnyLibraryId", token_api_auth, 
		"language", webhook_api, "registerNewUser", "aiEnabled" FROM "Tenant" WHERE id = $1`

	var tenant tenant.Tenant
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

func (t *TenantRepository) FindBunnyInfoByID(tenantID string) (*tenant.Tenant, error) {
	query := `SELECT id, "bunnyLibraryApiKey", "bunnyLibraryId"
				FROM "Tenant" WHERE id = $1`

	var tenant tenant.Tenant
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

func (t *TenantRepository) FindTenantByToken(ctx context.Context, token string) (*tenant.Tenant, error) {

	//TODO: create index to token_api_auth
	query := `
  SELECT id, name 
  FROM "Tenant" 
  WHERE token_api_auth = $1 
  LIMIT 1
`

	var tenant tenant.Tenant
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

func (t *TenantRepository) FindAllWithAIEnabled(ctx context.Context) ([]*tenant.Tenant, error) {
	query := `
		SELECT id, name, "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"
		FROM "Tenant"
		WHERE "aiEnabled" = true
	`

	rows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		t.log.Error("Error finding tenants with AI enabled: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding tenants with AI enabled",
		}
	}
	defer rows.Close()

	tenants := make([]*tenant.Tenant, 0)
	for rows.Next() {
		var tenant tenant.Tenant
		var bunnyLibraryID sql.NullString
		var bunnyLibraryApiKey sql.NullString

		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.AIEnabled,
			&bunnyLibraryID,
			&bunnyLibraryApiKey,
		)
		if err != nil {
			t.log.Error("Error scanning tenant: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning tenant",
			}
		}

		if bunnyLibraryID.Valid {
			tenant.BunnyLibraryID = &bunnyLibraryID.String
		}
		if bunnyLibraryApiKey.Valid {
			tenant.BunnyLibraryApiKey = &bunnyLibraryApiKey.String
		}

		tenants = append(tenants, &tenant)
	}

	if err = rows.Err(); err != nil {
		t.log.Error("Error iterating tenants: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating tenants",
		}
	}

	return tenants, nil
}

func NewTenantRepository(db *sql.DB, log ports.Logger) tenant2.TenantRepository {
	return &TenantRepository{
		db:  db,
		log: log,
	}
}
