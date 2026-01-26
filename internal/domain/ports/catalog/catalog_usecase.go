package catalog

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/catalog"
)

type CatalogUseCase interface {
	GetCatalog(ctx context.Context, tenantID string) (*catalog.CatalogResponse, error)
	GetVitrine(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*catalog.VitrineDetailResponse, error)
	GetCourse(ctx context.Context, courseID, tenantID string, includeChildren bool) (*catalog.CourseDetailResponse, error)
	GetModule(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*catalog.ModuleDetailResponse, error)
	GetLesson(ctx context.Context, lessonID, tenantID string) (*catalog.LessonDetailResponse, error)
}
