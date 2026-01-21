package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/auth"
	auth2 "github.com/memberclass-backend-golang/internal/domain/dto/response/auth"
	auth3 "github.com/memberclass-backend-golang/internal/domain/ports/auth"
	"github.com/memberclass-backend-golang/internal/domain/ports/tenant"
	"github.com/memberclass-backend-golang/internal/domain/ports/user"
	"golang.org/x/crypto/bcrypt"

	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

const (
	magicLinkKey = "%s://%s/login?token=%s&email=%s&isReset=false"
)

type AuthUseCaseImpl struct {
	userRepository   user.UserRepository
	tenantRepository tenant.TenantRepository
	cache            ports.Cache
	logger           ports.Logger
}

func NewAuthUseCase(
	userRepository user.UserRepository,
	tenantRepository tenant.TenantRepository,
	cache ports.Cache,
	logger ports.Logger,
) auth3.AuthUseCase {
	return &AuthUseCaseImpl{
		userRepository:   userRepository,
		tenantRepository: tenantRepository,
		cache:            cache,
		logger:           logger,
	}
}

func (uc *AuthUseCaseImpl) GenerateMagicLink(ctx context.Context, req auth.AuthRequest, tenantID string) (*auth2.AuthResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: err.Error(),
		}
	}

	cacheKey := fmt.Sprintf("auth_cache:%s:%s", tenantID, req.Email)
	cached, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		return &auth2.AuthResponse{
			OK:   true,
			Link: cached,
		}, nil
	}

	user, err := uc.userRepository.FindByEmail(req.Email)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrUserNotFound) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Usuário não encontrado",
			}
		}
		return nil, err
	}

	belongs, err := uc.userRepository.BelongsToTenant(user.ID, tenantID)
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error checking user tenant membership",
		}
	}

	if !belongs {
		return nil, &memberclasserrors.MemberClassError{
			Code:    404,
			Message: "Usuário não encontrado no tenant",
		}
	}

	token, err := uc.generateMagicToken()
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error generating magic token",
		}
	}

	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), 10)
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error hashing magic token",
		}
	}

	validUntil := time.Now().Add(7 * 24 * time.Hour)
	err = uc.userRepository.UpdateMagicToken(ctx, user.ID, string(tokenHash), validUntil)
	if err != nil {
		return nil, err
	}

	link, err := uc.buildLoginLink(tenantID, token, req.Email)
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error building login link",
		}
	}

	err = uc.cache.Set(ctx, cacheKey, link, 300*time.Second)
	if err != nil {
		uc.logger.Error("Error caching auth response: " + err.Error())
	}

	return &auth2.AuthResponse{
		OK:   true,
		Link: link,
	}, nil
}

func (uc *AuthUseCaseImpl) generateMagicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (uc *AuthUseCaseImpl) buildLoginLink(tenantID, token, email string) (string, error) {
	rootDomain := os.Getenv("PUBLIC_ROOT_DOMAIN")
	if rootDomain == "" {
		return "", errors.New("PUBLIC_ROOT_DOMAIN not set")
	}

	protocol := "https"
	if strings.Contains(rootDomain, "localhost") {
		protocol = "http"
	}

	tenant, err := uc.tenantRepository.FindByID(tenantID)
	if err != nil {
		return "", err
	}

	domain := ""
	if tenant.CustomDomain != nil && *tenant.CustomDomain != "" {
		domain = *tenant.CustomDomain
	} else {
		subdomain := "acessos"
		if tenant.SubDomain != nil && *tenant.SubDomain != "" {
			subdomain = *tenant.SubDomain
		}
		domain = fmt.Sprintf("%s.%s", subdomain, rootDomain)
	}

	link := fmt.Sprintf(magicLinkKey, protocol, domain, token, email)
	return link, nil
}
