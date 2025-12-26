package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type CommentRepository interface {
	Update(ctx context.Context, commentID, answer string, published bool) (*entities.Comment, error)
	FindByIDAndTenant(ctx context.Context, commentID, tenantID string) (*entities.Comment, error)
	FindByIDAndTenantWithDetails(ctx context.Context, commentID, tenantID string) (*dto.CommentResponse, error)
	FindAllByTenant(ctx context.Context, tenantID string, req *request.GetCommentsRequest) ([]*dto.CommentResponse, int64, error)
}
