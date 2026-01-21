package user

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user/activity"
)

type UserActivityUseCase interface {
	GetUserActivities(ctx context.Context, req user.GetActivitiesRequest, tenantID string) (*user2.ActivityResponse, error)
}
