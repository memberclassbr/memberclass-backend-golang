package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type AITenantUseCase interface {
	GetTenantsWithAIEnabled(ctx context.Context) (*response.AITenantsResponse, error)
}

