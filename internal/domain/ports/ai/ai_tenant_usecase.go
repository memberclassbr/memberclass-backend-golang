package ai

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
)

type AITenantUseCase interface {
	GetTenantsWithAIEnabled(ctx context.Context) (*ai.AITenantsResponse, error)
}
