package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UserInformationsUseCase interface {
	GetUserInformations(ctx context.Context, req request.GetUserInformationsRequest, tenantID string) (*response.UserInformationsResponse, error)
}

