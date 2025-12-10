package constants

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type contextKey string

const TenantContextKey contextKey = "tenant"

func GetTenantFromContext(ctx context.Context) *entities.Tenant {
	tenant, ok := ctx.Value(TenantContextKey).(*entities.Tenant)
	if !ok {
		return nil
	}
	return tenant
}
