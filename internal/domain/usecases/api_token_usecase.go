package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type ApiTokenTenantUseCase struct {
	tenantRepository ports.TenantRepository
	Logger           ports.Logger
}

func NewApiTokenTenantUseCase(tenantRepository ports.TenantRepository, logger ports.Logger) ports.ApiTokenUseCase {
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

func (uc *ApiTokenTenantUseCase) ValidateToken(ctx context.Context, token string) (*entities.Tenant, error) {
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
