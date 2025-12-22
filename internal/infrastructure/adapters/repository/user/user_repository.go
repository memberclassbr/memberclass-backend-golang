package user

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
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

func (r *UserRepository) FindPurchasesByUserAndTenant(ctx context.Context, userID, tenantID string, purchaseTypes []string, page, limit int) ([]response.UserPurchaseData, int64, error) {
	offset := (page - 1) * limit

	typesFilter := []string{"purchase", "refund"}
	if len(purchaseTypes) > 0 {
		typesFilter = purchaseTypes
	}

	query := `
		WITH filtered AS (
			SELECT id, "createdAt", type
			FROM "UserEvent"
			WHERE "usersOnTenantsUserId" = $1 
			  AND "usersOnTenantsTenantId" = $2
			  AND type = ANY($3)
		),
		paginated AS (
			SELECT * FROM filtered
			ORDER BY "createdAt" DESC
			LIMIT $4 OFFSET $5
		)
		SELECT 
			p.id, 
			p.type,
			TO_CHAR(p."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.000"Z"') as "createdAt",
			TO_CHAR(p."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.000"Z"') as "updatedAt",
			(SELECT COUNT(*) FROM filtered) as total_count
		FROM paginated p
	`

	rows, err := r.db.QueryContext(ctx, query, userID, tenantID, pq.Array(typesFilter), limit, offset)
	if err != nil {
		r.log.Error("Error finding purchases: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding purchases",
		}
	}
	defer rows.Close()

	purchases := make([]response.UserPurchaseData, 0)
	var total int64

	for rows.Next() {
		var purchase response.UserPurchaseData
		if err := rows.Scan(
			&purchase.ID,
			&purchase.Type,
			&purchase.CreatedAt,
			&purchase.UpdatedAt,
			&total,
		); err != nil {
			r.log.Error("Error scanning purchase: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning purchase",
			}
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating purchases: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating purchases",
		}
	}

	if len(purchases) == 0 {
		return purchases, 0, nil
	}

	return purchases, total, nil
}

