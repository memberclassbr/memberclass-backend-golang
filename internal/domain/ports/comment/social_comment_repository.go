package comment

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
)

type SocialCommentRepository interface {
	Create(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) (string, error)
	FindByID(ctx context.Context, postID string) (*PostInfo, error)
	Update(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) error
}

type PostInfo struct {
	ID     string
	UserID string
}
