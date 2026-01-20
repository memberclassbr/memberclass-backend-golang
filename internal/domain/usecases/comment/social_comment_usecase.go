package comment

import (
	"context"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/social"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/comment"
	"github.com/memberclass-backend-golang/internal/domain/ports/topic"
	"github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type SocialCommentUseCase struct {
	logger            ports.Logger
	userRepo          user.UserRepository
	socialCommentRepo comment.SocialCommentRepository
	topicRepo         topic.TopicRepository
}

func NewSocialCommentUseCase(logger ports.Logger, userRepo user.UserRepository, socialCommentRepo comment.SocialCommentRepository, topicRepo topic.TopicRepository) comment.SocialCommentUseCase {
	return &SocialCommentUseCase{
		logger:            logger,
		userRepo:          userRepo,
		socialCommentRepo: socialCommentRepo,
		topicRepo:         topicRepo,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForPost = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
	ErrPostNotFound                     = errors.New("Post não encontrado")
	ErrTopicNotFound                    = errors.New("Tópico não existe")
	ErrPermissionDenied                 = errors.New("Você não tem autorização para fazer esta ação")
	ErrNoAccessToTopic                  = errors.New("Você não tem acesso para publicar neste tópico")
)

func (uc *SocialCommentUseCase) CreateOrUpdatePost(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) (*social.SocialCommentResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	belongs, err := uc.userRepo.BelongsToTenant(req.UserID, tenantID)
	if err != nil {
		return nil, err
	}

	if !belongs {
		return nil, ErrUserNotFoundOrNotInTenantForPost
	}

	if req.PostID != "" {
		return uc.updatePost(ctx, req, tenantID)
	}

	return uc.createPost(ctx, req, tenantID)
}

func (uc *SocialCommentUseCase) updatePost(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) (*social.SocialCommentResponse, error) {
	post, err := uc.socialCommentRepo.FindByID(ctx, req.PostID)
	if err != nil {
		return nil, err
	}

	if post == nil {
		return nil, ErrPostNotFound
	}

	isOwner, err := uc.userRepo.IsUserOwner(ctx, req.UserID, tenantID)
	if err != nil {
		return nil, err
	}

	if !isOwner && post.UserID != req.UserID {
		return nil, ErrPermissionDenied
	}

	err = uc.socialCommentRepo.Update(ctx, req, tenantID)
	if err != nil {
		return nil, err
	}

	return &social.SocialCommentResponse{
		OK: true,
		ID: req.PostID,
	}, nil
}

func (uc *SocialCommentUseCase) createPost(ctx context.Context, req comments.CreateSocialCommentRequest, tenantID string) (*social.SocialCommentResponse, error) {
	isOwner, err := uc.userRepo.IsUserOwner(ctx, req.UserID, tenantID)
	if err != nil {
		return nil, err
	}

	topic, err := uc.topicRepo.FindByIDWithDeliveries(ctx, req.TopicID)
	if err != nil {
		return nil, err
	}

	if topic == nil {
		return nil, ErrTopicNotFound
	}

	if topic.OnlyAdmin && !isOwner {
		return nil, ErrNoAccessToTopic
	}

	if len(topic.DeliveryIDs) > 0 && !isOwner {
		userDeliveryIDs, err := uc.userRepo.GetUserDeliveryIDs(ctx, req.UserID)
		if err != nil {
			return nil, err
		}

		hasAccess := false
		for _, ud := range userDeliveryIDs {
			for _, td := range topic.DeliveryIDs {
				if ud == td {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}

		if !hasAccess {
			return nil, ErrNoAccessToTopic
		}
	}

	postID, err := uc.socialCommentRepo.Create(ctx, req, tenantID)
	if err != nil {
		return nil, err
	}

	return &social.SocialCommentResponse{
		OK: true,
		ID: postID,
	}, nil
}
