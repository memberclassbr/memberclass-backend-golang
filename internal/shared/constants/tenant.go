package constants

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
)

type contextKey string

const TenantContextKey contextKey = "tenant"

func GetTenantFromContext(ctx context.Context) *tenant.Tenant {
	tenant, ok := ctx.Value(TenantContextKey).(*tenant.Tenant)
	if !ok {
		return nil
	}
	return tenant
}
