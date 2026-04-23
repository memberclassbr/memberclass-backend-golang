package user

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
)

type UserActivityRepository interface {
	FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]activity.AccessData, int64, error)
}
