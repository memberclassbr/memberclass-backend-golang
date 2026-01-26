package vitrine

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	vitrineports "github.com/memberclass-backend-golang/internal/domain/ports/vitrine"
)

type VitrineUseCaseImpl struct {
	vitrineRepository vitrineports.VitrineRepository
}

func NewVitrineUseCase(vitrineRepository vitrineports.VitrineRepository) vitrineports.VitrineUseCase {
	return &VitrineUseCaseImpl{
		vitrineRepository: vitrineRepository,
	}
}

func (uc *VitrineUseCaseImpl) GetVitrines(ctx context.Context, tenantID string) (*vitrine.VitrineResponse, error) {
	if tenantID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "tenantId é obrigatório",
		}
	}

	return uc.vitrineRepository.GetVitrinesByTenant(ctx, tenantID)
}

func (uc *VitrineUseCaseImpl) GetVitrine(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*vitrine.VitrineDetailResponse, error) {
	if vitrineID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "vitrineId é obrigatório",
		}
	}

	if tenantID == "" {
		tenant := constants.GetTenantFromContext(ctx)
		if tenant == nil {
			return nil, &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "Token de API inválido",
			}
		}
		tenantID = tenant.ID
	}

	return uc.vitrineRepository.GetVitrineByID(ctx, vitrineID, tenantID, includeChildren)
}

func (uc *VitrineUseCaseImpl) GetCourse(ctx context.Context, courseID, tenantID string, includeChildren bool) (*vitrine.CourseDetailResponse, error) {
	if courseID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "courseId é obrigatório",
		}
	}

	if tenantID == "" {
		tenant := constants.GetTenantFromContext(ctx)
		if tenant == nil {
			return nil, &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "Token de API inválido",
			}
		}
		tenantID = tenant.ID
	}

	return uc.vitrineRepository.GetCourseByID(ctx, courseID, tenantID, includeChildren)
}

func (uc *VitrineUseCaseImpl) GetModule(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*vitrine.ModuleDetailResponse, error) {
	if moduleID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "moduleId é obrigatório",
		}
	}

	if tenantID == "" {
		tenant := constants.GetTenantFromContext(ctx)
		if tenant == nil {
			return nil, &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "Token de API inválido",
			}
		}
		tenantID = tenant.ID
	}

	return uc.vitrineRepository.GetModuleByID(ctx, moduleID, tenantID, includeChildren)
}

func (uc *VitrineUseCaseImpl) GetLesson(ctx context.Context, lessonID, tenantID string) (*vitrine.LessonDetailResponse, error) {
	if lessonID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "lessonId é obrigatório",
		}
	}

	if tenantID == "" {
		tenant := constants.GetTenantFromContext(ctx)
		if tenant == nil {
			return nil, &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "Token de API inválido",
			}
		}
		tenantID = tenant.ID
	}

	return uc.vitrineRepository.GetLessonByID(ctx, lessonID, tenantID)
}
