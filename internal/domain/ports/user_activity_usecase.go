package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UserActivityUseCase interface {
	GetUserActivities(ctx context.Context, req request.GetActivitiesRequest, tenantID string) (*response.ActivityResponse, error)
}
