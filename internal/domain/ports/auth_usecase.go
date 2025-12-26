package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type AuthUseCase interface {
	GenerateMagicLink(ctx context.Context, req request.AuthRequest, tenantID string) (*response.AuthResponse, error)
}

