package user

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/dto/response/user"
)

type UserInformationsUseCase interface {
	GetUserInformations(ctx context.Context, req user.GetUserInformationsRequest, tenantID string) (*user2.UserInformationsResponse, error)
}
