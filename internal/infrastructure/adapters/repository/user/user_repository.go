package user

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

func (r *UserRepository) FindUserInformations(ctx context.Context, tenantID string, email string, page, limit int) ([]response.UserInformation, int64, error) {
	offset := (page - 1) * limit

	var query string
	var queryParams []interface{}

	if email != "" {
		query = `
			WITH users_base AS (
				SELECT 
					uot."userId",
					uot."assignedAt",
					u.id,
					u.email,
					uot.name
				FROM "UsersOnTenants" uot
				JOIN "User" u ON u.id = uot."userId"
				WHERE uot."tenantId" = $1 AND u.email = $4
				ORDER BY uot."assignedAt" DESC
				LIMIT $2 OFFSET $3
			),
			paid_users AS (
				SELECT DISTINCT "usersOnTenantsUserId" as user_id
				FROM "UserEvent"
				WHERE "usersOnTenantsUserId" IN (SELECT "userId" FROM users_base)
				  AND "usersOnTenantsTenantId" = $1
				  AND type = 'purchase'
				  AND value >= 0
			),
			last_access AS (
				SELECT DISTINCT ON (sl.user_id) sl.user_id as "userId", sl."createdAt" as "updatedAt"
				FROM "SystemLog" sl
				WHERE sl.user_id IN (SELECT "userId" FROM users_base)
				ORDER BY sl.user_id, sl."createdAt" DESC
			)
			SELECT 
				ub."userId",
				ub.email,
				ub.name,
				(pu.user_id IS NOT NULL) as is_paid,
				la."updatedAt" as last_access
			FROM users_base ub
			LEFT JOIN paid_users pu ON pu.user_id = ub."userId"
			LEFT JOIN last_access la ON la."userId" = ub."userId"
		`
		queryParams = []interface{}{tenantID, limit, offset, email}
	} else {
		query = `
			WITH users_base AS (
				SELECT 
					uot."userId",
					uot."assignedAt",
					u.id,
					u.email,
					uot.name
				FROM "UsersOnTenants" uot
				JOIN "User" u ON u.id = uot."userId"
				WHERE uot."tenantId" = $1
				ORDER BY uot."assignedAt" DESC
				LIMIT $2 OFFSET $3
			),
			paid_users AS (
				SELECT DISTINCT "usersOnTenantsUserId" as user_id
				FROM "UserEvent"
				WHERE "usersOnTenantsUserId" IN (SELECT "userId" FROM users_base)
				  AND "usersOnTenantsTenantId" = $1
				  AND type = 'purchase'
				  AND value >= 0
			),
			last_access AS (
				SELECT DISTINCT ON (sl.user_id) sl.user_id as "userId", sl."createdAt" as "updatedAt"
				FROM "SystemLog" sl
				WHERE sl.user_id IN (SELECT "userId" FROM users_base)
				ORDER BY sl.user_id, sl."createdAt" DESC
			)
			SELECT 
				ub."userId",
				ub.email,
				ub.name,
				(pu.user_id IS NOT NULL) as is_paid,
				la."updatedAt" as last_access
			FROM users_base ub
			LEFT JOIN paid_users pu ON pu.user_id = ub."userId"
			LEFT JOIN last_access la ON la."userId" = ub."userId"
		`
		queryParams = []interface{}{tenantID, limit, offset}
	}

	countQuery := `
		SELECT COUNT(*)
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		WHERE uot."tenantId" = $1
	`
	countParams := []interface{}{tenantID}

	if email != "" {
		countQuery += ` AND u.email = $2`
		countParams = append(countParams, email)
	}

	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, countParams...).Scan(&total)
	if err != nil {
		r.log.Error("Error counting users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting users",
		}
	}

	rows, err := r.db.QueryContext(ctx, query, queryParams...)
	if err != nil {
		r.log.Error("Error finding user informations: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding user informations",
		}
	}
	defer rows.Close()

	userMap := make(map[string]*response.UserInformation)
	userIDs := make([]string, 0)

	for rows.Next() {
		var userID, userEmail, userName string
		var isPaid bool
		var lastAccess sql.NullTime

		if err := rows.Scan(&userID, &userEmail, &userName, &isPaid, &lastAccess); err != nil {
			r.log.Error("Error scanning user information: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning user information",
			}
		}

		var lastAccessStr *string
		if lastAccess.Valid {
			formatted := lastAccess.Time.Format("2006-01-02T15:04:05.000Z")
			lastAccessStr = &formatted
		}

		userMap[userID] = &response.UserInformation{
			UserID:     userID,
			Email:      userEmail,
			IsPaid:     isPaid,
			Deliveries: make([]response.DeliveryInfo, 0),
			LastAccess: lastAccessStr,
		}
		userIDs = append(userIDs, userID)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating user informations: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating user informations",
		}
	}

	if len(userIDs) == 0 {
		result := make([]response.UserInformation, 0)
		return result, total, nil
	}

	deliveriesQuery := `
		SELECT uod."userId", uod."deliveryId", uod."assignedAt", d.name as delivery_name
		FROM "UserOnDelivery" uod
		JOIN "Delivery" d ON d.id = uod."deliveryId"
		WHERE uod."userId" = ANY($1) AND d."tenantId" = $2
		UNION ALL
		SELECT mod."memberId", mod."deliveryId", mod."assignedAt", d.name
		FROM "MemberOnDelivery" mod
		JOIN "Delivery" d ON d.id = mod."deliveryId"
		WHERE mod."memberId" = ANY($1) AND mod."tenantId" = $2
		ORDER BY "assignedAt" DESC
	`

	deliveryRows, err := r.db.QueryContext(ctx, deliveriesQuery, pq.Array(userIDs), tenantID)
	if err != nil {
		r.log.Error("Error finding deliveries: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding deliveries",
		}
	}
	defer deliveryRows.Close()

	for deliveryRows.Next() {
		var userID, deliveryID, deliveryName string
		var accessDate time.Time

		if err := deliveryRows.Scan(&userID, &deliveryID, &accessDate, &deliveryName); err != nil {
			r.log.Error("Error scanning delivery: " + err.Error())
			continue
		}

		if user, exists := userMap[userID]; exists {
			user.Deliveries = append(user.Deliveries, response.DeliveryInfo{
				ID:         deliveryID,
				Name:       deliveryName,
				AccessDate: accessDate.Format("2006-01-02T15:04:05.000Z"),
			})
		}
	}

	result := make([]response.UserInformation, 0, len(userMap))
	for _, userID := range userIDs {
		if user, exists := userMap[userID]; exists {
			result = append(result, *user)
		}
	}

	return result, total, nil
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

func (r *UserRepository) GetUserDeliveryIDs(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT "deliveryId" 
		FROM "UserOnDelivery" 
		WHERE "userId" = $1
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
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

