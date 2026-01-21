package user

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/purchase"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/purchases"
)

type UserPurchaseUseCase interface {
	GetUserPurchases(ctx context.Context, req purchase.GetUserPurchasesRequest, tenantID string) (*purchases.UserPurchasesResponse, error)
}
