package ai

import (
	"context"

	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	ai3 "github.com/memberclass-backend-golang/internal/domain/ports/ai"
	"github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

// AITenantUseCaseImpl exposes the small slice of "AI tenant" behaviour the
// legacy admin dashboard still needs (GetTenantsWithAIEnabled). The old
// ProcessLessonsTenant entry point moved into the transcription slice.
type AITenantUseCaseImpl struct {
	tenantRepository tenant.TenantRepository
	logger           ports.Logger
}

func NewAITenantUseCase(
	tenantRepository tenant.TenantRepository,
	logger ports.Logger,
) ai3.AITenantUseCase {
	return &AITenantUseCaseImpl{
		tenantRepository: tenantRepository,
		logger:           logger,
	}
}

func (uc *AITenantUseCaseImpl) GetTenantsWithAIEnabled(ctx context.Context) (*ai2.AITenantsResponse, error) {
	tenants, err := uc.tenantRepository.FindAllWithAIEnabled(ctx)
	if err != nil {
		return nil, err
	}

	tenantData := make([]ai2.AITenantData, len(tenants))
	for i, tenant := range tenants {
		var bunnyLibraryID *string
		var bunnyLibraryApiKey *string

		if tenant.BunnyLibraryID != nil {
			bunnyLibraryID = tenant.BunnyLibraryID
		}
		if tenant.BunnyLibraryApiKey != nil {
			bunnyLibraryApiKey = tenant.BunnyLibraryApiKey
		}

		tenantData[i] = ai2.AITenantData{
			ID:                 tenant.ID,
			Name:               tenant.Name,
			AIEnabled:          tenant.AIEnabled,
			BunnyLibraryID:     bunnyLibraryID,
			BunnyLibraryApiKey: bunnyLibraryApiKey,
		}
	}

	return &ai2.AITenantsResponse{
		Tenants: tenantData,
		Total:   len(tenantData),
	}, nil
}
