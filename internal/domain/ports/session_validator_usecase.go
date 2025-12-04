package ports

type SessionValidatorUseCase interface {
	ValidateUserExists(userID string) error
	ValidateUserBelongsToTenant(userID string, tenantID string) error
}

