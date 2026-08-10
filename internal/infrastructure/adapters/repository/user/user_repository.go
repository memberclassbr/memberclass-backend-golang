package user

import (
	"context"
	"database/sql"
	"errors"
	"time"

	userentities "github.com/memberclass-backend-golang/internal/domain/entities/user"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	userports "github.com/memberclass-backend-golang/internal/domain/ports/user"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

type UserRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewUserRepository(db *sql.DB, log ports.Logger) userports.UserRepository {
	return &UserRepository{
		db:  db,
		log: log,
	}
}

func (r *UserRepository) FindByID(userID string) (*userentities.User, error) {
	query := `SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = $1`

	var user userentities.User
	err := r.db.QueryRow(query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Phone,
		&user.Email,
		&user.EmailVerified,
		&user.Image,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Referrals,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrUserNotFound
		}
		r.log.Error("Error finding user: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding user",
		}
	}

	return &user, nil
}

func (r *UserRepository) FindByEmail(email string) (*userentities.User, error) {
	query := `SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE email = $1`

	var user userentities.User
	err := r.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Phone,
		&user.Email,
		&user.EmailVerified,
		&user.Image,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Referrals,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrUserNotFound
		}
		r.log.Error("Error finding user by email: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding user by email",
		}
	}

	return &user, nil
}

func (r *UserRepository) ExistsByID(userID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM "User" WHERE id = $1)`

	var exists bool
	err := r.db.QueryRow(query, userID).Scan(&exists)
	if err != nil {
		r.log.Error("Error checking user existence: " + err.Error())
		return false, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error checking user existence",
		}
	}

	return exists, nil
}

func (r *UserRepository) BelongsToTenant(userID string, tenantID string) (bool, error) {
	query := `SELECT EXISTS(
		SELECT 1 FROM "UsersOnTenants" 
		WHERE "userId" = $1 AND "tenantId" = $2
	)`

	var belongs bool
	err := r.db.QueryRow(query, userID, tenantID).Scan(&belongs)
	if err != nil {
		r.log.Error("Error checking user tenant membership: " + err.Error())
		return false, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error checking user tenant membership",
		}
	}

	return belongs, nil
}

func (r *UserRepository) IsUserOwner(ctx context.Context, userID, tenantID string) (bool, error) {
	query := `
		SELECT "userId" 
		FROM "UsersOnTenants" 
		WHERE "userId" = $1 AND "tenantId" = $2 AND role = 'owner' 
		LIMIT 1
	`

	var id string
	err := r.db.QueryRowContext(ctx, query, userID, tenantID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		r.log.Error("Error checking if user is owner: " + err.Error())
		return false, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error checking if user is owner",
		}
	}

	return true, nil
}

func (r *UserRepository) GetUserDeliveryIDs(ctx context.Context, userID string, tenantID string) ([]string, error) {
	query := `
		SELECT "deliveryId"
		FROM "MemberOnDelivery"
		WHERE "memberId" = $1 AND "tenantId" = $2
	`

	rows, err := r.db.QueryContext(ctx, query, userID, tenantID)
	if err != nil {
		r.log.Error("Error getting user delivery IDs: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting user delivery IDs",
		}
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			r.log.Error("Error scanning delivery ID: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning delivery ID",
			}
		}
		ids = append(ids, id)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating delivery IDs: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating delivery IDs",
		}
	}

	return ids, nil
}

func (r *UserRepository) UpdateMagicToken(ctx context.Context, userID string, tokenHash string, validUntil time.Time) error {
	query := `
		UPDATE "User"
		SET "magicToken" = $1, "magicTokenValidUntil" = $2, "updatedAt" = NOW()
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, tokenHash, validUntil, userID)
	if err != nil {
		r.log.Error("Error updating magic token: " + err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating magic token",
		}
	}

	return nil
}
