package ports

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UserActivityRepository interface {
	FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]response.AccessData, int64, error)
	GetActivitySummaryByEmail(ctx context.Context, email string) (*response.ActivitySummaryResponse, error)
	GetUsersWithActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]response.UserActivitySummary, int64, error)
	GetUsersWithoutActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]response.UserActivitySummary, int64, error)
}
