package user_activity

import (
	"context"
	"database/sql"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type UserActivityRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewUserActivityRepository(db *sql.DB, log ports.Logger) ports.UserActivityRepository {
	return &UserActivityRepository{
		db:  db,
		log: log,
	}
}

func (r *UserActivityRepository) FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]response.AccessData, int64, error) {
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

	activities := make([]response.AccessData, 0)
	for rows.Next() {
		var access response.AccessData
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
