package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type PostSocialRepository interface {
	Create(ctx context.Context, email, platform, url, tenantID string) (*response.PostSocialResponse, error)
	FindByEmail(ctx context.Context, email, tenantID string) ([]*response.PostSocialResponse, error)
}

