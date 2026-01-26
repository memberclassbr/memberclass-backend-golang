package catalog

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/catalog"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	catalogports "github.com/memberclass-backend-golang/internal/domain/ports/catalog"
)

type CatalogUseCaseImpl struct {
	catalogRepository catalogports.CatalogRepository
}

func NewCatalogUseCase(catalogRepository catalogports.CatalogRepository) catalogports.CatalogUseCase {
	return &CatalogUseCaseImpl{
		catalogRepository: catalogRepository,
	}
}

func (uc *CatalogUseCaseImpl) GetCatalog(ctx context.Context, tenantID string) (*catalog.CatalogResponse, error) {
	if tenantID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "tenantId é obrigatório",
		}
	}

	return uc.catalogRepository.GetCatalogByTenant(ctx, tenantID)
}

func (uc *CatalogUseCaseImpl) GetVitrine(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*catalog.VitrineDetailResponse, error) {
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

	return uc.catalogRepository.GetVitrineByID(ctx, vitrineID, tenantID, includeChildren)
}

func (uc *CatalogUseCaseImpl) GetCourse(ctx context.Context, courseID, tenantID string, includeChildren bool) (*catalog.CourseDetailResponse, error) {
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

	return uc.catalogRepository.GetCourseByID(ctx, courseID, tenantID, includeChildren)
}

func (uc *CatalogUseCaseImpl) GetModule(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*catalog.ModuleDetailResponse, error) {
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

	return uc.catalogRepository.GetModuleByID(ctx, moduleID, tenantID, includeChildren)
}

func (uc *CatalogUseCaseImpl) GetLesson(ctx context.Context, lessonID, tenantID string) (*catalog.LessonDetailResponse, error) {
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

	return uc.catalogRepository.GetLessonByID(ctx, lessonID, tenantID)
}
