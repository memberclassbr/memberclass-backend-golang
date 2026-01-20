package tenant

import (
	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type TenantGetTenantBunnyCredentialsUseCase interface {
	Execute(tenantID string) (*dto.TenantBunnyCredentials, error)
}
