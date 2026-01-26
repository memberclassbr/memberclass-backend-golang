package vitrine

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
)

type VitrineUseCase interface {
	GetVitrines(ctx context.Context, tenantID string) (*vitrine.VitrineResponse, error)
	GetVitrine(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*vitrine.VitrineDetailResponse, error)
	GetCourse(ctx context.Context, courseID, tenantID string, includeChildren bool) (*vitrine.CourseDetailResponse, error)
	GetModule(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*vitrine.ModuleDetailResponse, error)
	GetLesson(ctx context.Context, lessonID, tenantID string) (*vitrine.LessonDetailResponse, error)
}
