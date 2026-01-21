package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/auth"
	tenant2 "github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

type ApiTokenTenantUseCase struct {
	tenantRepository tenant2.TenantRepository
	Logger           ports.Logger
}

func NewApiTokenTenantUseCase(tenantRepository tenant2.TenantRepository, logger ports.Logger) auth.ApiTokenUseCase {
	return &ApiTokenTenantUseCase{
		tenantRepository: tenantRepository,
		Logger:           logger,
	}
}

func (uc *ApiTokenTenantUseCase) GenerateToken(ctx context.Context, tenantID string) (string, error) {

	if tenantID == "" {
		return "", errors.New("tenantID is required")
	}

	_, err := uc.tenantRepository.FindByID(tenantID)
	if err != nil {
		return "", err
	}

	token := uc.generateHash(ctx)

	if token != "" {
		err = uc.tenantRepository.UpdateTokenApiAuth(ctx, tenantID, token)
		if err != nil {
			return "", err
		}
	}

	return token, nil

}

func (uc *ApiTokenTenantUseCase) ValidateToken(ctx context.Context, token string) (*tenant.Tenant, error) {
	if token == "" {
		return nil, errors.New("token is required")
	}

	tenant, err := uc.tenantRepository.FindTenantByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	return tenant, nil

}

func (uc *ApiTokenTenantUseCase) generateHash(ctx context.Context) string {
	token := uuid.NewString()

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	return tokenHash
}
