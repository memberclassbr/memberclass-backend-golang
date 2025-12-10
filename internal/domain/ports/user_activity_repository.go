package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UserActivityRepository interface {
	FindActivitiesByEmail(ctx context.Context, email string, page, limit int) ([]response.AccessData, int64, error)
}
