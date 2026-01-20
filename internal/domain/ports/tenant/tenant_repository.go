package tenant

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
)

type TenantRepository interface {
	FindByID(tenantID string) (*tenant.Tenant, error)
	FindBunnyInfoByID(tenantID string) (*tenant.Tenant, error)
	FindTenantByToken(ctx context.Context, token string) (*tenant.Tenant, error)
	UpdateTokenApiAuth(ctx context.Context, tenantID, tokenHash string) error
	FindAllWithAIEnabled(ctx context.Context) ([]*tenant.Tenant, error)
}
