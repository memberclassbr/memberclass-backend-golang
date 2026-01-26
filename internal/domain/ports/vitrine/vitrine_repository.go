package vitrine

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
)

type VitrineRepository interface {
	GetVitrinesByTenant(ctx context.Context, tenantID string) (*vitrine.VitrineResponse, error)
	GetVitrineByID(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*vitrine.VitrineDetailResponse, error)
	GetCourseByID(ctx context.Context, courseID, tenantID string, includeChildren bool) (*vitrine.CourseDetailResponse, error)
	GetModuleByID(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*vitrine.ModuleDetailResponse, error)
	GetLessonByID(ctx context.Context, lessonID, tenantID string) (*vitrine.LessonDetailResponse, error)
}
