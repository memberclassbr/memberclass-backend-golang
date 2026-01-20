package auth

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
)

type ApiTokenUseCase interface {
	GenerateToken(ctx context.Context, tenantID string) (string, error)
	ValidateToken(ctx context.Context, token string) (*tenant.Tenant, error)
}
