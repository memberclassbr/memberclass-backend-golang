package user

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
)

type UserActivityRepository interface {
	FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]activity.AccessData, int64, error)
	GetActivitySummaryByEmail(ctx context.Context, email string) (*user.ActivitySummaryResponse, error)
	GetUsersWithActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]user.UserActivitySummary, int64, error)
	GetUsersWithoutActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]user.UserActivitySummary, int64, error)
}
