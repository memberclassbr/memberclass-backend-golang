package user_activity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	user2 "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type UserActivityRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewUserActivityRepository(db *sql.DB, log ports.Logger) user2.UserActivityRepository {
	return &UserActivityRepository{
		db:  db,
		log: log,
	}
}

func (r *UserActivityRepository) FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]activity.AccessData, int64, error) {
	offset := (page - 1) * limit

	query := `
		SELECT 
			TO_CHAR(sl."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as data
		FROM "SystemLog" sl
		JOIN "User" u ON sl.user_id = u.id
		WHERE u.email = $1
		ORDER BY sl."createdAt" DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, email, limit, offset)
	if err != nil {
		r.log.Error("Error finding activities: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding activities",
		}
	}
	defer rows.Close()

	activities := make([]activity.AccessData, 0)
	for rows.Next() {
		var access activity.AccessData
		if err := rows.Scan(&access.Data); err != nil {
			r.log.Error("Error scanning activity: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning activity",
			}
		}
		activities = append(activities, access)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating activities: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating activities",
		}
	}

	countQuery := `
		SELECT COUNT(*) 
		FROM "SystemLog" sl
		JOIN "User" u ON sl.user_id = u.id
		WHERE u.email = $1
	`

	var total int64
	err = r.db.QueryRowContext(ctx, countQuery, email).Scan(&total)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return activities, 0, nil
		}
		r.log.Error("Error counting activities: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting activities",
		}
	}

	return activities, total, nil
}

func (r *UserActivityRepository) GetActivitySummaryByEmail(ctx context.Context, email string) (*user.ActivitySummaryResponse, error) {
	query := `
		SELECT 
			COUNT(DISTINCT sl.id) as total_activities,
			MAX(sl."createdAt") as last_activity,
			COUNT(DISTINCT CASE WHEN sl.action = 'lesson_completed' THEN sl.id END) as lessons_completed,
			COUNT(DISTINCT CASE WHEN sl.action = 'course_accessed' THEN sl.id END) as courses_accessed
		FROM "SystemLog" sl
		JOIN "User" u ON sl.user_id = u.id
		WHERE u.email = $1
	`

	var totalActivities, lessonsCompleted, coursesAccessed int
	var lastActivity sql.NullTime

	_ = r.db.QueryRowContext(ctx, query, email).Scan(
		&totalActivities,
		&lastActivity,
		&lessonsCompleted,
		&coursesAccessed,
	)

	// This method is kept for backward compatibility but returns empty structure
	// The new GetUsersWithActivity/GetUsersWithoutActivity should be used instead
	return &user.ActivitySummaryResponse{
		Users:      []user.UserActivitySummary{},
		Pagination: dto.PaginationMeta{},
	}, nil
}

func (r *UserActivityRepository) GetUsersWithActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]user.UserActivitySummary, int64, error) {
	offset := (page - 1) * limit

	// First, get user IDs with activity in the period
	userIDsQuery := `
		SELECT DISTINCT "usersOnTenantsUserId"
		FROM "UserEvent"
		WHERE "usersOnTenantsTenantId" = $1
		  AND "createdAt" >= $2
		  AND "createdAt" <= $3
		  AND "usersOnTenantsUserId" IS NOT NULL
	`

	rows, err := r.db.QueryContext(ctx, userIDsQuery, tenantID, startDate, endDate)
	if err != nil {
		r.log.Error("Error getting users with activity: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting users with activity",
		}
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			r.log.Error("Error scanning user ID: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning user ID",
			}
		}
		userIDs = append(userIDs, userID)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating user IDs: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating user IDs",
		}
	}

	if len(userIDs) == 0 {
		return []user.UserActivitySummary{}, 0, nil
	}

	// Get last access for each user
	lastAccessQuery := `
		SELECT "usersOnTenantsUserId", MAX("createdAt") as ultimo_acesso
		FROM "UserEvent"
		WHERE "usersOnTenantsUserId" = ANY($1)
		  AND "usersOnTenantsTenantId" = $2
		  AND "createdAt" >= $3
		  AND "createdAt" <= $4
		GROUP BY "usersOnTenantsUserId"
	`

	lastAccessRows, err := r.db.QueryContext(ctx, lastAccessQuery, pq.Array(userIDs), tenantID, startDate, endDate)
	if err != nil {
		r.log.Error("Error getting last access: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting last access",
		}
	}
	defer lastAccessRows.Close()

	lastAccessMap := make(map[string]time.Time)
	for lastAccessRows.Next() {
		var userID string
		var lastAccess time.Time
		if err := lastAccessRows.Scan(&userID, &lastAccess); err != nil {
			r.log.Error("Error scanning last access: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning last access",
			}
		}
		lastAccessMap[userID] = lastAccess
	}

	// Get paginated users
	usersQuery := `
		SELECT u.email, uot."userId"
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		WHERE uot."tenantId" = $1
		  AND uot."userId" = ANY($2)
		ORDER BY u.email ASC
		LIMIT $3 OFFSET $4
	`

	userRows, err := r.db.QueryContext(ctx, usersQuery, tenantID, pq.Array(userIDs), limit, offset)
	if err != nil {
		r.log.Error("Error getting paginated users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting paginated users",
		}
	}
	defer userRows.Close()

	var users []user.UserActivitySummary
	for userRows.Next() {
		var email, userID string
		if err := userRows.Scan(&email, &userID); err != nil {
			r.log.Error("Error scanning user: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning user",
			}
		}

		var ultimoAcesso *string
		if lastAccess, ok := lastAccessMap[userID]; ok {
			formatted := lastAccess.Format("2006-01-02T15:04:05.000Z")
			ultimoAcesso = &formatted
		}

		users = append(users, user.UserActivitySummary{
			Email:        email,
			UltimoAcesso: ultimoAcesso,
		})
	}

	if err = userRows.Err(); err != nil {
		r.log.Error("Error iterating users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating users",
		}
	}

	// Count total
	var totalCount int64
	countQuery := `
		SELECT COUNT(*)
		FROM "UsersOnTenants"
		WHERE "tenantId" = $1
		  AND "userId" = ANY($2)
	`
	err = r.db.QueryRowContext(ctx, countQuery, tenantID, pq.Array(userIDs)).Scan(&totalCount)
	if err != nil {
		r.log.Error("Error counting users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting users",
		}
	}

	return users, totalCount, nil
}

func (r *UserActivityRepository) GetUsersWithoutActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]user.UserActivitySummary, int64, error) {
	offset := (page - 1) * limit

	query := `
		SELECT u.email
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		WHERE uot."tenantId" = $1
		  AND uot."userId" NOT IN (
		    SELECT DISTINCT "usersOnTenantsUserId"
		    FROM "UserEvent"
		    WHERE "usersOnTenantsTenantId" = $1
		      AND "createdAt" >= $2
		      AND "createdAt" <= $3
		      AND "usersOnTenantsUserId" IS NOT NULL
		  )
		ORDER BY u.email ASC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, startDate, endDate, limit, offset)
	if err != nil {
		r.log.Error("Error getting users without activity: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting users without activity",
		}
	}
	defer rows.Close()

	var users []user.UserActivitySummary
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			r.log.Error("Error scanning user: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning user",
			}
		}

		users = append(users, user.UserActivitySummary{
			Email:        email,
			UltimoAcesso: nil,
		})
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating users",
		}
	}

	// Count total
	var totalCount int64
	countQuery := `
		SELECT COUNT(*)
		FROM "UsersOnTenants" uot
		WHERE uot."tenantId" = $1
		  AND uot."userId" NOT IN (
		    SELECT DISTINCT "usersOnTenantsUserId"
		    FROM "UserEvent"
		    WHERE "usersOnTenantsTenantId" = $1
		      AND "createdAt" >= $2
		      AND "createdAt" <= $3
		      AND "usersOnTenantsUserId" IS NOT NULL
		  )
	`
	err = r.db.QueryRowContext(ctx, countQuery, tenantID, startDate, endDate).Scan(&totalCount)
	if err != nil {
		r.log.Error("Error counting users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting users",
		}
	}

	return users, totalCount, nil
}
