package sso

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	ssoports "github.com/memberclass-backend-golang/internal/domain/ports/sso"
)

type SSORepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewSSORepository(db *sql.DB, log ports.Logger) ssoports.SSORepository {
	return &SSORepository{
		db:  db,
		log: log,
	}
}

func (r *SSORepository) UpdateSSOToken(ctx context.Context, userID, tenantID, tokenHash string, validUntil time.Time) error {
	query := `
		UPDATE "UsersOnTenants"
		SET "ssoToken" = $1, "ssoTokenValidUntil" = $2, "ssoTokenUsedAt" = NULL, "ssoTokenIP" = NULL
		WHERE "userId" = $3 AND "tenantId" = $4
	`

	result, err := r.db.ExecContext(ctx, query, tokenHash, validUntil, userID, tenantID)
	if err != nil {
		r.log.Error("Error updating SSO token: " + err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating SSO token",
		}
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		r.log.Error("Error checking rows affected: " + err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error checking rows affected",
		}
	}

	if rowsAffected == 0 {
		return &memberclasserrors.MemberClassError{
			Code:    404,
			Message: "user not found in tenant",
		}
	}

	return nil
}

func (r *SSORepository) ValidateAndConsumeSSOToken(ctx context.Context, tokenHash, ip string) (*response.ValidateSSOTokenResponse, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.log.Error("Error starting transaction: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error starting transaction",
		}
	}
	defer tx.Rollback()

	query := `
		SELECT 
			uot."userId", 
			uot."tenantId",
			uot."ssoTokenValidUntil"::timestamp,
			uot."ssoTokenUsedAt"::timestamp,
			u.email,
			uot.name,
			u.phone,
			t.name as tenant_name
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		JOIN "Tenant" t ON t.id = uot."tenantId"
		WHERE uot."ssoToken" = $1
		FOR UPDATE
	`

	var userID, tenantID, email, tenantName string
	var name, phone *string
	var validUntil time.Time
	var usedAt sql.NullTime

	err = tx.QueryRowContext(ctx, query, tokenHash).Scan(
		&userID,
		&tenantID,
		&validUntil,
		&usedAt,
		&email,
		&name,
		&phone,
		&tenantName,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "token inválido",
			}
		}
		r.log.Error("Error validating SSO token: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error validating SSO token",
		}
	}

	if usedAt.Valid {
		return nil, &memberclasserrors.MemberClassError{
			Code:    401,
			Message: "token já foi utilizado",
		}
	}

	now := time.Now().UTC()
	if now.After(validUntil.UTC()) {
		return nil, &memberclasserrors.MemberClassError{
			Code:    401,
			Message: "token expirado",
		}
	}

	updateQuery := `
		UPDATE "UsersOnTenants"
		SET "ssoTokenUsedAt" = $1, "ssoTokenIP" = $2
		WHERE "userId" = $3 AND "tenantId" = $4
	`

	now = time.Now().UTC()
	_, err = tx.ExecContext(ctx, updateQuery, now, ip, userID, tenantID)
	if err != nil {
		r.log.Error("Error marking token as used: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error marking token as used",
		}
	}

	if err := tx.Commit(); err != nil {
		r.log.Error("Error committing transaction: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error committing transaction",
		}
	}

	document, err := r.GetUserDocument(ctx, userID)
	if err != nil {
		r.log.Error("Error getting user document: " + err.Error())
	}

	return &response.ValidateSSOTokenResponse{
		User: response.SSOUserData{
			ID:       userID,
			Email:    email,
			Name:     name,
			Phone:    phone,
			Document: document,
		},
		Tenant: response.SSOTenantData{
			ID:   tenantID,
			Name: tenantName,
		},
	}, nil
}

func (r *SSORepository) GetUserDocument(ctx context.Context, userID string) (*string, error) {
	query := `
		SELECT document 
		FROM "UsersOnTenants" 
		WHERE "userId" = $1 AND document IS NOT NULL
		LIMIT 1
	`

	var document sql.NullString
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&document)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		r.log.Error("Error getting user document: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting user document",
		}
	}

	if !document.Valid {
		return nil, nil
	}

	return &document.String, nil
}
