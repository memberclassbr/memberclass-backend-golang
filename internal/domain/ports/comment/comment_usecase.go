package comment

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/comment"
)

type CommentUseCase interface {
	UpdateAnswer(ctx context.Context, commentID, tenantID string, req comments.UpdateCommentRequest) (*comment.CommentResponse, error)
	GetComments(ctx context.Context, tenantID string, req *comments.GetCommentsRequest) (*comment.CommentsPaginationResponse, error)
}
