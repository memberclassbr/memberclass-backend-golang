package sso

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type SSORepository interface {
	UpdateSSOToken(ctx context.Context, userID, tenantID, tokenHash string, validUntil time.Time) error
	ValidateAndConsumeSSOToken(ctx context.Context, tokenHash, ip string) (*response.ValidateSSOTokenResponse, error)
	GetUserDocument(ctx context.Context, userID string) (*string, error)
}
