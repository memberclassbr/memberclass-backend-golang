package user

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/purchases"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/user"
	user2 "github.com/memberclass-backend-golang/internal/domain/entities/user"
)

type UserRepository interface {
	FindByID(userID string) (*user2.User, error)
	FindByEmail(email string) (*user2.User, error)
	ExistsByID(userID string) (bool, error)
	BelongsToTenant(userID string, tenantID string) (bool, error)
	FindPurchasesByUserAndTenant(ctx context.Context, userID, tenantID string, purchaseTypes []string, page, limit int) ([]purchases.UserPurchaseData, int64, error)
	FindUserInformations(ctx context.Context, tenantID string, email string, page, limit int) ([]user.UserInformation, int64, error)
	IsUserOwner(ctx context.Context, userID, tenantID string) (bool, error)
	GetUserDeliveryIDs(ctx context.Context, userID string) ([]string, error)
	UpdateMagicToken(ctx context.Context, userID string, tokenHash string, validUntil time.Time) error
}
