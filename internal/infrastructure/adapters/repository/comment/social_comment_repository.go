package comment

import (
	"context"
	"database/sql"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/utils"
)

type SocialCommentRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewSocialCommentRepository(db *sql.DB, log ports.Logger) ports.SocialCommentRepository {
	return &SocialCommentRepository{
		db:  db,
		log: log,
	}
}

func (r *SocialCommentRepository) Create(ctx context.Context, req request.CreateSocialCommentRequest, tenantID string) (string, error) {
	id := utils.GenerateCUID()

	query := `
		INSERT INTO "Post" (id, "topicId", title, content, published, image, "videoEmbed", "userId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, true, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`

	err := r.db.QueryRowContext(ctx, query, id, req.TopicID, req.Title, req.Content, req.Image, req.VideoEmbed, req.UserID).Scan(&id)
	if err != nil {
		r.log.Error("Error creating post: " + err.Error())
		return "", &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating post",
		}
	}

	return id, nil
}

func (r *SocialCommentRepository) FindByID(ctx context.Context, postID string) (*ports.PostInfo, error) {
	query := `
		SELECT id, "userId" 
		FROM "Post" 
		WHERE id = $1
	`

	var post ports.PostInfo
	err := r.db.QueryRowContext(ctx, query, postID).Scan(&post.ID, &post.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		r.log.Error("Error finding post: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding post",
		}
	}

	return &post, nil
}

func (r *SocialCommentRepository) Update(ctx context.Context, req request.CreateSocialCommentRequest, tenantID string) error {
	query := `
		UPDATE "Post" 
		SET title = $2, content = $3, image = $4, "videoEmbed" = $5, "updatedAt" = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, req.PostID, req.Title, req.Content, req.Image, req.VideoEmbed)
	if err != nil {
		r.log.Error("Error updating post: " + err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating post",
		}
	}

	return nil
}


