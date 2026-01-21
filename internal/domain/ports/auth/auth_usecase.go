package auth

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/auth"
	auth2 "github.com/memberclass-backend-golang/internal/domain/dto/response/auth"
)

type AuthUseCase interface {
	GenerateMagicLink(ctx context.Context, req auth.AuthRequest, tenantID string) (*auth2.AuthResponse, error)
}
