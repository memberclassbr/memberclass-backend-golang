package usecases

import (
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type TenantGetTenantBunnyCredentialsUseCase struct {
	rp  ports.TenantRepository
	log ports.Logger
}

func NewTenantGetTenantBunnyCredentialsUseCase(rp ports.TenantRepository, log ports.Logger) ports.TenantGetTenantBunnyCredentialsUseCase {
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
	tenantBunnyCredentials.BunnyLibraryID = tenant.BunnyLibraryID
	tenantBunnyCredentials.BunnyLibraryApiKey = tenant.BunnyLibraryApiKey

	return &tenantBunnyCredentials, nil
}
