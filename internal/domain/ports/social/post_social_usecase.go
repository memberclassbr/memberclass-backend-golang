package social

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/social"
	social2 "github.com/memberclass-backend-golang/internal/domain/dto/response/social"
)

type PostSocialUseCase interface {
	CreatePostSocial(ctx context.Context, req social.CreatePostSocialRequest, tenantID string) (*social2.PostSocialResponse, error)
}
