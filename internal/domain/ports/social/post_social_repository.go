package social

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/social"
)

type PostSocialRepository interface {
	Create(ctx context.Context, email, platform, url, tenantID string) (*social.PostSocialResponse, error)
	FindByEmail(ctx context.Context, email, tenantID string) ([]*social.PostSocialResponse, error)
}
