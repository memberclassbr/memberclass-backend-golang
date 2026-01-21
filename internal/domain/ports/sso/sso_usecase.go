package sso

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/sso"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type SSOUseCase interface {
	GenerateSSOToken(ctx context.Context, req sso.GenerateSSOTokenRequest, externalURL string) (*response.GenerateSSOTokenResponse, error)
	ValidateSSOToken(ctx context.Context, token, ip string) (*response.ValidateSSOTokenResponse, error)
}
