package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type SocialCommentUseCase interface {
	CreateOrUpdatePost(ctx context.Context, req request.CreateSocialCommentRequest, tenantID string) (*response.SocialCommentResponse, error)
}

