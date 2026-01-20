package comment

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/comment"
	comment2 "github.com/memberclass-backend-golang/internal/domain/entities/comment"
)

type CommentRepository interface {
	Update(ctx context.Context, commentID, answer string, published bool) (*comment2.Comment, error)
	FindByIDAndTenant(ctx context.Context, commentID, tenantID string) (*comment2.Comment, error)
	FindByIDAndTenantWithDetails(ctx context.Context, commentID, tenantID string) (*comment.CommentResponse, error)
	FindAllByTenant(ctx context.Context, tenantID string, req *comments.GetCommentsRequest) ([]*comment.CommentResponse, int64, error)
}
