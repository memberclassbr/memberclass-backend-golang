package ports

import "github.com/memberclass-backend-golang/internal/domain/entities"

type UserRepository interface {
	FindByID(userID string) (*entities.User, error)
	ExistsByID(userID string) (bool, error)
	BelongsToTenant(userID string, tenantID string) (bool, error)
}

