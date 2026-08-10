package user

import (
	"context"
	"time"

	user2 "github.com/memberclass-backend-golang/internal/domain/entities/user"
)

type UserRepository interface {
	FindByID(userID string) (*user2.User, error)
	FindByEmail(email string) (*user2.User, error)
	ExistsByID(userID string) (bool, error)
	BelongsToTenant(userID string, tenantID string) (bool, error)
	IsUserOwner(ctx context.Context, userID, tenantID string) (bool, error)
	GetUserDeliveryIDs(ctx context.Context, userID string, tenantID string) ([]string, error)
	UpdateMagicToken(ctx context.Context, userID string, tokenHash string, validUntil time.Time) error
}
