package catalog

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/catalog"
)

type CatalogRepository interface {
	GetCatalogByTenant(ctx context.Context, tenantID string) (*catalog.CatalogResponse, error)
	GetVitrineByID(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*catalog.VitrineDetailResponse, error)
	GetCourseByID(ctx context.Context, courseID, tenantID string, includeChildren bool) (*catalog.CourseDetailResponse, error)
	GetModuleByID(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*catalog.ModuleDetailResponse, error)
	GetLessonByID(ctx context.Context, lessonID, tenantID string) (*catalog.LessonDetailResponse, error)
}
