package bunny

import (
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

type TenantGetTenantBunnyCredentialsUseCase struct {
	rp  tenant.TenantRepository
	log ports.Logger
}

func NewTenantGetTenantBunnyCredentialsUseCase(rp tenant.TenantRepository, log ports.Logger) tenant.TenantGetTenantBunnyCredentialsUseCase {
	return &TenantGetTenantBunnyCredentialsUseCase{
		rp:  rp,
		log: log,
	}
}

func (t *TenantGetTenantBunnyCredentialsUseCase) Execute(tenantID string) (*dto.TenantBunnyCredentials, error) {

	if tenantID == "" {
		return nil, memberclasserrors.ErrTenantIDEmpty
	}

	tenant, err := t.rp.FindBunnyInfoByID(tenantID)
	if err != nil {
		return nil, err
	}

	var tenantBunnyCredentials dto.TenantBunnyCredentials

	tenantBunnyCredentials.TenantID = tenant.ID
	if tenant.BunnyLibraryID != nil {
		tenantBunnyCredentials.BunnyLibraryID = *tenant.BunnyLibraryID
	}
	if tenant.BunnyLibraryApiKey != nil {
		tenantBunnyCredentials.BunnyLibraryApiKey = *tenant.BunnyLibraryApiKey
	}

	return &tenantBunnyCredentials, nil
}
