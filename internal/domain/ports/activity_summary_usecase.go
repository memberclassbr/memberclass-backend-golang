package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type ActivitySummaryUseCase interface {
	GetActivitySummary(ctx context.Context, req request.GetActivitySummaryRequest, tenantID string) (*response.ActivitySummaryResponse, error)
}

