package usecases

import (
	"context"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

var (
	ErrCommentNotFound = errors.New("comentário não encontrado ou não pertence a este tenant")
	ErrAnswerRequired  = errors.New("campo 'answer' é obrigatório e deve ser uma string")
)

type CommentUseCase struct {
	logger            ports.Logger
	commentRepository ports.CommentRepository
}

func NewCommentUseCase(logger ports.Logger, commentRepository ports.CommentRepository) ports.CommentUseCase {
	return &CommentUseCase{
		logger:            logger,
		commentRepository: commentRepository,
	}
}

func (uc *CommentUseCase) UpdateAnswer(ctx context.Context, commentID, tenantID string, req dto.UpdateCommentRequest) (*dto.CommentResponse, error) {
	if req.Answer == "" {
		return nil, ErrAnswerRequired
	}

	existing, err := uc.commentRepository.FindByIDAndTenant(ctx, commentID, tenantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrCommentNotFound
	}

	published := false
	if req.Published != nil {
		published = *req.Published
	}

	_, err = uc.commentRepository.Update(ctx, commentID, req.Answer, published)
	if err != nil {
		return nil, err
	}

	response, err := uc.commentRepository.FindByIDAndTenantWithDetails(ctx, commentID, tenantID)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (uc *CommentUseCase) GetComments(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) (*dto.CommentsPaginationResponse, error) {
	comments, total, err := uc.commentRepository.FindAllByTenant(ctx, tenantID, pagination)
	if err != nil {
		return nil, err
	}

	response := dto.NewCommentsPaginationResponse(comments, total, pagination)
	return response, nil
}
