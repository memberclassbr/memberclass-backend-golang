package user

import (
	"database/sql"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type UserRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewUserRepository(db *sql.DB, log ports.Logger) ports.UserRepository {
	return &UserRepository{
		db:  db,
		log: log,
	}
}

func (r *UserRepository) FindByID(userID string) (*entities.User, error) {
	query := `SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE id = $1`

	var user entities.User
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

func (r *UserRepository) FindByEmail(email string) (*entities.User, error) {
	query := `SELECT id, username, phone, email, "emailVerified", image, 
		"createdAt", "updatedAt", referrals 
		FROM "User" WHERE email = $1`

	var user entities.User
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

