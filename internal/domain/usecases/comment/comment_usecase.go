package comment

import (
	"context"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/comment"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	comment2 "github.com/memberclass-backend-golang/internal/domain/ports/comment"
	"github.com/memberclass-backend-golang/internal/domain/ports/user"
)

var (
	ErrCommentNotFound = errors.New("comentário não encontrado ou não pertence a este tenant")
	ErrAnswerRequired  = errors.New("campo 'answer' é obrigatório e deve ser uma string")
	ErrUserNotFound    = errors.New("usuário não encontrado")
	ErrUserNotInTenant = errors.New("usuário não está associado a este tenant")
)

type CommentUseCase struct {
	logger            ports.Logger
	commentRepository comment2.CommentRepository
	userRepository    user.UserRepository
}

func NewCommentUseCase(logger ports.Logger, commentRepository comment2.CommentRepository, userRepository user.UserRepository) comment2.CommentUseCase {
	return &CommentUseCase{
		logger:            logger,
		commentRepository: commentRepository,
		userRepository:    userRepository,
	}
}

func (uc *CommentUseCase) UpdateAnswer(ctx context.Context, commentID, tenantID string, req comments.UpdateCommentRequest) (*comment.CommentResponse, error) {
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

func (uc *CommentUseCase) GetComments(ctx context.Context, tenantID string, req *comments.GetCommentsRequest) (*comment.CommentsPaginationResponse, error) {
	// Validar request
	if err := req.Validate(); err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: err.Error(),
		}
	}

	// Validar email se fornecido
	if req.Email != nil && *req.Email != "" {
		user, err := uc.userRepository.FindByEmail(*req.Email)
		if err != nil {
			var memberClassErr *memberclasserrors.MemberClassError
			if errors.As(err, &memberClassErr) && memberClassErr.Code == 404 {
				return nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Usuário não encontrado",
				}
			}
			return nil, err
		}

		belongs, err := uc.userRepository.BelongsToTenant(user.ID, tenantID)
		if err != nil {
			return nil, err
		}

		if !belongs {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Usuário não está associado a este tenant",
			}
		}
	}

	comments, total, err := uc.commentRepository.FindAllByTenant(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	response := comment.NewCommentsPaginationResponse(comments, total, req.Page, req.Limit)
	return response, nil
}
