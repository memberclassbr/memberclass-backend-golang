package pdf_processor

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities/lessons"
)

type PdfProcessorUseCase interface {
	ProcessLesson(ctx context.Context, lessonID string) (*dto.ProcessResult, error)
	ProcessAllPendingLessons(ctx context.Context, limit int) (*dto.BatchProcessResult, error)
	RetryFailedAssets(ctx context.Context) error
	CleanupOrphanedPages(ctx context.Context) error
	RegeneratePDF(ctx context.Context, lessonID string) error
	ConvertPdfToImages(pdfURL string) ([]string, error)
	CreateOrUpdatePDFAsset(ctx context.Context, lessonID, pdfURL string) (*lessons.LessonPDFAsset, error)
	SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error)
	ValidateLessonHasPDF(ctx context.Context, lessonID string) error
	GetLessonWithPDFAsset(ctx context.Context, lessonID string) (*lessons.Lesson, error)
	GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*lessons.LessonPDFPage, error)
}
