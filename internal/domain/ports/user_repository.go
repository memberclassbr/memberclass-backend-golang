package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type UserRepository interface {
	FindByID(userID string) (*entities.User, error)
	FindByEmail(email string) (*entities.User, error)
	ExistsByID(userID string) (bool, error)
	BelongsToTenant(userID string, tenantID string) (bool, error)
	FindPurchasesByUserAndTenant(ctx context.Context, userID, tenantID string, purchaseTypes []string, page, limit int) ([]response.UserPurchaseData, int64, error)
}

