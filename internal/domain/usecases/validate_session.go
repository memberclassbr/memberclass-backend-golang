package usecases

import (
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type ValidateSessionUseCase struct {
	userRepository ports.UserRepository
	logger         ports.Logger
}

func NewValidateSessionUseCase(userRepository ports.UserRepository, logger ports.Logger) ports.SessionValidatorUseCase {
	return &ValidateSessionUseCase{
		userRepository: userRepository,
		logger:         logger,
	}
}

func (uc *ValidateSessionUseCase) ValidateUserExists(userID string) error {
	if userID == "" {
		return memberclasserrors.ErrUserNotFound
	}

	exists, err := uc.userRepository.ExistsByID(userID)
	if err != nil {
		uc.logger.Error("Failed to check user existence: " + err.Error())
		return err
	}

	if !exists {
		uc.logger.Debug("User not found in database: " + userID)
		return memberclasserrors.ErrUserNotFound
	}

	return nil
}

func (uc *ValidateSessionUseCase) ValidateUserBelongsToTenant(userID string, tenantID string) error {
	if userID == "" || tenantID == "" {
		return memberclasserrors.ErrUserNotInTenant
	}

	belongs, err := uc.userRepository.BelongsToTenant(userID, tenantID)
	if err != nil {
		uc.logger.Error("Failed to check tenant membership: " + err.Error())
		return err
	}

	if !belongs {
		uc.logger.Debug("User does not belong to tenant: " + tenantID)
		return memberclasserrors.ErrUserNotInTenant
	}

	return nil
}

