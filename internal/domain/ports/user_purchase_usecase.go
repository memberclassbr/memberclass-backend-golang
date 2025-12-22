package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UserPurchaseUseCase interface {
	GetUserPurchases(ctx context.Context, req request.GetUserPurchasesRequest, tenantID string) (*response.UserPurchasesResponse, error)
}
