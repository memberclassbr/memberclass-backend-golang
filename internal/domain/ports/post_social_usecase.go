package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type PostSocialUseCase interface {
	CreatePostSocial(ctx context.Context, req request.CreatePostSocialRequest, tenantID string) (*response.PostSocialResponse, error)
}

