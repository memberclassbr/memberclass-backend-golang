package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type ApiTokenUseCase interface {
	GenerateToken(ctx context.Context, tenantID string) (string, error)
	ValidateToken(ctx context.Context, token string) (*entities.Tenant, error)
}
