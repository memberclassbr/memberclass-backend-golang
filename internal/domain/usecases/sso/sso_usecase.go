package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/sso"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	ssoports "github.com/memberclass-backend-golang/internal/domain/ports/sso"
	userports "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type SSOUseCase struct {
	ssoRepo  ssoports.SSORepository
	userRepo userports.UserRepository
	logger   ports.Logger
}

func NewSSOUseCase(
	ssoRepo ssoports.SSORepository,
	userRepo userports.UserRepository,
	logger ports.Logger,
) ssoports.SSOUseCase {
	return &SSOUseCase{
		ssoRepo:  ssoRepo,
		userRepo: userRepo,
		logger:   logger,
	}
}

func (u *SSOUseCase) GenerateSSOToken(ctx context.Context, req sso.GenerateSSOTokenRequest, externalURL string) (*response.GenerateSSOTokenResponse, error) {
	exists, err := u.userRepo.ExistsByID(req.UserID)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, &memberclasserrors.MemberClassError{
			Code:    404,
			Message: "usuário não encontrado",
		}
	}

	belongsToTenant, err := u.userRepo.BelongsToTenant(req.UserID, req.TenantID)
	if err != nil {
		return nil, err
	}

	if !belongsToTenant {
		return nil, &memberclasserrors.MemberClassError{
			Code:    403,
			Message: "usuário não pertence ao tenant",
		}
	}

	token, err := u.generateRandomToken(32)
	if err != nil {
		u.logger.Error("Error generating random token: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao gerar token",
		}
	}

	tokenHash := u.generateSHA256Hash(token)
	validUntil := time.Now().UTC().Add(5 * time.Minute)

	err = u.ssoRepo.UpdateSSOToken(ctx, req.UserID, req.TenantID, tokenHash, validUntil)
	if err != nil {
		return nil, err
	}

	redirectURL, err := u.buildRedirectURL(externalURL, token)
	if err != nil {
		u.logger.Error("Error building redirect URL: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "URL externa inválida",
		}
	}

	return &response.GenerateSSOTokenResponse{
		Token:         token,
		RedirectURL:   redirectURL,
		ExpiresInSecs: 300,
	}, nil
}

func (u *SSOUseCase) ValidateSSOToken(ctx context.Context, token, ip string) (*response.ValidateSSOTokenResponse, error) {
	if token == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "token é obrigatório",
		}
	}

	tokenHash := u.generateSHA256Hash(token)

	return u.ssoRepo.ValidateAndConsumeSSOToken(ctx, tokenHash, ip)
}

func (u *SSOUseCase) generateRandomToken(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = charset[bytes[i]%byte(len(charset))]
	}
	return string(bytes), nil
}

func (u *SSOUseCase) generateSHA256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

func (u *SSOUseCase) buildRedirectURL(externalURL, token string) (string, error) {
	parsedURL, err := url.Parse(externalURL)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("token-mc", token)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}
