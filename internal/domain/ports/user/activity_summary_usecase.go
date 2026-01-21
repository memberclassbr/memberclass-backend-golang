package user

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user"
)

type ActivitySummaryUseCase interface {
	GetActivitySummary(ctx context.Context, req user.GetActivitySummaryRequest, tenantID string) (*user2.ActivitySummaryResponse, error)
}
