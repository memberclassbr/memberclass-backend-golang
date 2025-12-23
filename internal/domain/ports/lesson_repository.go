package ports

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
)

type LessonRepository interface {
	GetByID(ctx context.Context, id string) (*entities.Lesson, error)
	GetByIDWithPDFAsset(ctx context.Context, id string) (*entities.Lesson, error)
	GetPendingPDFLessons(ctx context.Context, limit int) ([]*entities.Lesson, error)
	Update(ctx context.Context, lesson *entities.Lesson) error

	GetPDFAssetByLessonID(ctx context.Context, lessonID string) (*entities.LessonPDFAsset, error)
	CreatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error
	UpdatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error
	UpdatePDFAssetStatus(ctx context.Context, assetID, status string, totalPages *int, errorMsg *string) error
	GetFailedPDFAssets(ctx context.Context) ([]*entities.LessonPDFAsset, error)

	CreatePDFPage(ctx context.Context, page *entities.LessonPDFPage) error
	GetPDFPageByAssetAndNumber(ctx context.Context, assetID string, pageNumber int) (*entities.LessonPDFPage, error)
	GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*entities.LessonPDFPage, error)
	DeletePDFPage(ctx context.Context, pageID string) error
	DeletePDFPagesByAssetID(ctx context.Context, assetID string) error
	FindCompletedLessonsByEmail(ctx context.Context, userID, tenantID string, startDate, endDate time.Time, courseID string, page, limit int) ([]response.CompletedLesson, int64, error)
}
