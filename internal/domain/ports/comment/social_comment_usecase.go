package comment

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/social"
)

type SocialCommentUseCase interface {
	CreateOrUpdatePost(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) (*social.SocialCommentResponse, error)
}
