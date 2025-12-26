package usecases

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type AITenantUseCaseImpl struct {
	tenantRepository ports.TenantRepository
	logger           ports.Logger
}

func NewAITenantUseCase(
	tenantRepository ports.TenantRepository,
	logger ports.Logger,
) ports.AITenantUseCase {
	return &AITenantUseCaseImpl{
		tenantRepository: tenantRepository,
		logger:           logger,
	}
}

func (uc *AITenantUseCaseImpl) GetTenantsWithAIEnabled(ctx context.Context) (*response.AITenantsResponse, error) {
	tenants, err := uc.tenantRepository.FindAllWithAIEnabled(ctx)
	if err != nil {
		return nil, err
	}

	tenantData := make([]response.AITenantData, len(tenants))
	for i, tenant := range tenants {
		var bunnyLibraryID *string
		var bunnyLibraryApiKey *string

		if tenant.BunnyLibraryID != nil {
			bunnyLibraryID = tenant.BunnyLibraryID
		}
		if tenant.BunnyLibraryApiKey != nil {
			bunnyLibraryApiKey = tenant.BunnyLibraryApiKey
		}

		tenantData[i] = response.AITenantData{
			ID:                 tenant.ID,
			Name:               tenant.Name,
			AIEnabled:          tenant.AIEnabled,
			BunnyLibraryID:     bunnyLibraryID,
			BunnyLibraryApiKey: bunnyLibraryApiKey,
		}
	}

	return &response.AITenantsResponse{
		Tenants: tenantData,
		Total:   len(tenantData),
	}, nil
}
