package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type TenantRepository interface {
	FindByID(tenantID string) (*entities.Tenant, error)
	FindBunnyInfoByID(tenantID string) (*entities.Tenant, error)
	FindTenantByToken(ctx context.Context, token string) (*entities.Tenant, error)
	UpdateTokenApiAuth(ctx context.Context, tenantID, tokenHash string) error
}
