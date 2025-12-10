package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type CommentUseCase interface {
	UpdateAnswer(ctx context.Context, commentID, tenantID string, req dto.UpdateCommentRequest) (*dto.CommentResponse, error)
	GetComments(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) (*dto.CommentsPaginationResponse, error)
}
