package ports

import "github.com/memberclass-backend-golang/internal/domain/entities"

type TenantRepository interface {
	FindByID(tenantID string) (*entities.Tenant, error)
	FindBunnyInfoByID(tenantID string) (*entities.Tenant, error)
}
