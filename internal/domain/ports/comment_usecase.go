package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
)

type CommentUseCase interface {
	UpdateAnswer(ctx context.Context, commentID, tenantID string, req request.UpdateCommentRequest) (*dto.CommentResponse, error)
	GetComments(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) (*dto.CommentsPaginationResponse, error)
}
