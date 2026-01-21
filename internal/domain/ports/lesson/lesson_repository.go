package lesson

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
	"github.com/memberclass-backend-golang/internal/domain/entities/lessons"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
)

type LessonRepository interface {
	GetByID(ctx context.Context, id string) (*lessons.Lesson, error)
	GetByIDWithPDFAsset(ctx context.Context, id string) (*lessons.Lesson, error)
	GetPendingPDFLessons(ctx context.Context, limit int) ([]*lessons.Lesson, error)
	Update(ctx context.Context, lesson *lessons.Lesson) error

	GetPDFAssetByLessonID(ctx context.Context, lessonID string) (*lessons.LessonPDFAsset, error)
	CreatePDFAsset(ctx context.Context, asset *lessons.LessonPDFAsset) error
	UpdatePDFAsset(ctx context.Context, asset *lessons.LessonPDFAsset) error
	UpdatePDFAssetStatus(ctx context.Context, assetID, status string, totalPages *int, errorMsg *string) error
	GetFailedPDFAssets(ctx context.Context) ([]*lessons.LessonPDFAsset, error)

	CreatePDFPage(ctx context.Context, page *lessons.LessonPDFPage) error
	GetPDFPageByAssetAndNumber(ctx context.Context, assetID string, pageNumber int) (*lessons.LessonPDFPage, error)
	GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*lessons.LessonPDFPage, error)
	DeletePDFPage(ctx context.Context, pageID string) error
	DeletePDFPagesByAssetID(ctx context.Context, assetID string) error
	FindCompletedLessonsByEmail(ctx context.Context, userID, tenantID string, startDate, endDate time.Time, courseID string, page, limit int) ([]lesson.CompletedLesson, int64, error)
	GetByIDWithTenant(ctx context.Context, lessonID string) (*lessons.Lesson, *tenant.Tenant, error)
	UpdateTranscriptionStatus(ctx context.Context, lessonID string, transcriptionCompleted bool) error
	GetLessonsWithHierarchyByTenant(ctx context.Context, tenantID string, onlyUnprocessed bool) ([]AILessonWithHierarchy, error)
}

type AILessonWithHierarchy struct {
	ID                     string
	Name                   string
	Slug                   string
	Type                   *string
	MediaURL               *string
	Thumbnail              *string
	Content                *string
	TranscriptionCompleted bool
	ModuleID               string
	ModuleName             string
	SectionID              string
	SectionName            string
	CourseID               string
	CourseName             string
	VitrineID              string
	VitrineName            string
}
