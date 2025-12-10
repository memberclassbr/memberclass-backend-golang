package lesson

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewLessonRepository(t *testing.T) {
	db, _, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := NewLessonRepository(db, mockLogger)

	assert.NotNil(t, repo)
	assert.IsType(t, &LessonRepository{}, repo)
}

func TestLessonRepository_GetByID_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mediaURL := "https://example.com/media.mp4"
	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "video",
		"test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	)

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, slug, name, 
		published, "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries", 
		thumbnail, content, "moduleId", "createdBy", "showDescriptionToggle", 
		"bannersTitle", "transcriptionCompleted" 
		FROM "Lesson" WHERE id = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetByID(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "lesson-123", *result.ID)
	assert.Equal(t, &mediaURL, result.MediaURL)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByID_NotFound(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, slug, name, 
		published, "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries", 
		thumbnail, content, "moduleId", "createdBy", "showDescriptionToggle", 
		"bannersTitle", "transcriptionCompleted" 
		FROM "Lesson" WHERE id = \$1`).
		WithArgs("lesson-123").
		WillReturnError(sql.ErrNoRows)

	result, err := repo.GetByID(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrLessonNotFound, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByID_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, slug, name, 
		published, "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries", 
		thumbnail, content, "moduleId", "createdBy", "showDescriptionToggle", 
		"bannersTitle", "transcriptionCompleted" 
		FROM "Lesson" WHERE id = \$1`).
		WithArgs("lesson-123").
		WillReturnError(errors.New("database error"))

	result, err := repo.GetByID(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFAssetByLessonID_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "lessonId", "sourcePdfUrl", "totalPages", "status", "error", "createdAt", "updatedAt",
	}).AddRow(
		"asset-123", "lesson-123", "https://example.com/source.pdf", 5, "completed", nil,
		time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE "lessonId" = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFAssetByLessonID(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "asset-123", result.ID)
	assert.Equal(t, "lesson-123", result.LessonID)
	assert.Equal(t, 5, *result.TotalPages)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFAssetByLessonID_NotFound(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE "lessonId" = \$1`).
		WithArgs("lesson-123").
		WillReturnError(sql.ErrNoRows)

	result, err := repo.GetPDFAssetByLessonID(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrPDFAssetNotFound, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_DeletePDFPage_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectExec(`DELETE FROM "LessonPdfPage" WHERE id = \$1`).
		WithArgs("page-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.DeletePDFPage(context.Background(), "page-123")

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_DeletePDFPagesByAssetID_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectExec(`DELETE FROM "LessonPdfPage" WHERE "assetId" = \$1`).
		WithArgs("asset-123").
		WillReturnResult(sqlmock.NewResult(2, 2))

	err := repo.DeletePDFPagesByAssetID(context.Background(), "asset-123")

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByID_NullMediaURL(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "video",
		"test-lesson", "Test Lesson", true, 1, nil, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	)

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, slug, name, 
		published, "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries", 
		thumbnail, content, "moduleId", "createdBy", "showDescriptionToggle", 
		"bannersTitle", "transcriptionCompleted" 
		FROM "Lesson" WHERE id = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetByID(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "lesson-123", *result.ID)
	assert.Nil(t, result.MediaURL)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByIDWithPDFAsset_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mediaURL := "https://example.com/media.mp4"
	rows := sqlmock.NewRows([]string{
		"l.id", "l.createdAt", "l.updatedAt", "l.access", "l.referenceAccess", "l.type",
		"l.slug", "l.name", "l.published", "l.order", "l.mediaUrl", "l.fullHdStatus",
		"l.fullHdUrl", "l.fullHdRetries", "l.thumbnail", "l.content", "l.moduleId",
		"l.createdBy", "l.showDescriptionToggle", "l.bannersTitle", "l.transcriptionCompleted",
		"p.id", "p.lessonId", "p.sourcePdfUrl", "p.totalPages", "p.status", "p.error",
		"p.createdAt", "p.updatedAt",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "video",
		"test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
		"asset-123", "lesson-123", "https://example.com/source.pdf", 5, "completed", nil,
		time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetByIDWithPDFAsset(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "lesson-123", *result.ID)
	assert.NotNil(t, result.PDFAsset)
	assert.Equal(t, "asset-123", result.PDFAsset.ID)
	assert.Equal(t, 5, *result.PDFAsset.TotalPages)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByIDWithPDFAsset_WithoutPDFAsset(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mediaURL := "https://example.com/media.mp4"
	rows := sqlmock.NewRows([]string{
		"l.id", "l.createdAt", "l.updatedAt", "l.access", "l.referenceAccess", "l.type",
		"l.slug", "l.name", "l.published", "l.order", "l.mediaUrl", "l.fullHdStatus",
		"l.fullHdUrl", "l.fullHdRetries", "l.thumbnail", "l.content", "l.moduleId",
		"l.createdBy", "l.showDescriptionToggle", "l.bannersTitle", "l.transcriptionCompleted",
		"p.id", "p.lessonId", "p.sourcePdfUrl", "p.totalPages", "p.status", "p.error",
		"p.createdAt", "p.updatedAt",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "video",
		"test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)

	mockDB.ExpectQuery(`SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetByIDWithPDFAsset(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "lesson-123", *result.ID)
	assert.Nil(t, result.PDFAsset)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByIDWithPDFAsset_NotFound(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = \$1`).
		WithArgs("lesson-123").
		WillReturnError(sql.ErrNoRows)

	result, err := repo.GetByIDWithPDFAsset(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrLessonNotFound, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByIDWithPDFAsset_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = \$1`).
		WithArgs("lesson-123").
		WillReturnError(errors.New("database error"))

	result, err := repo.GetByIDWithPDFAsset(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetByIDWithPDFAsset_WithNullTotalPagesAndError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mediaURL := "https://example.com/media.mp4"
	rows := sqlmock.NewRows([]string{
		"l.id", "l.createdAt", "l.updatedAt", "l.access", "l.referenceAccess", "l.type",
		"l.slug", "l.name", "l.published", "l.order", "l.mediaUrl", "l.fullHdStatus",
		"l.fullHdUrl", "l.fullHdRetries", "l.thumbnail", "l.content", "l.moduleId",
		"l.createdBy", "l.showDescriptionToggle", "l.bannersTitle", "l.transcriptionCompleted",
		"p.id", "p.lessonId", "p.sourcePdfUrl", "p.totalPages", "p.status", "p.error",
		"p.createdAt", "p.updatedAt",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "video",
		"test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
		"asset-123", "lesson-123", "https://example.com/source.pdf", nil, "processing", nil,
		time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetByIDWithPDFAsset(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.PDFAsset)
	assert.Nil(t, result.PDFAsset.TotalPages)
	assert.Nil(t, result.PDFAsset.Error)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPendingPDFLessons_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mediaURL := "https://example.com/lesson.pdf"
	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "pdf",
		"test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	).AddRow(
		"lesson-456", time.Now(), time.Now(), 1, "public", "pdf",
		"test-lesson-2", "Test Lesson 2", true, 2, "https://example.com/lesson2.pdf", "completed",
		"https://example.com/video2.mp4", 0, "https://example.com/thumb2.jpg",
		"Test content 2", "module-123", "user-123", true, "Test Banner 2", true,
	)

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN \(
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  \)
		ORDER BY "createdAt" ASC
		LIMIT \$1`).
		WithArgs(10).
		WillReturnRows(rows)

	result, err := repo.GetPendingPDFLessons(context.Background(), 10)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "lesson-123", *result[0].ID)
	assert.Equal(t, "lesson-456", *result[1].ID)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPendingPDFLessons_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN \(
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  \)
		ORDER BY "createdAt" ASC
		LIMIT \$1`).
		WithArgs(10).
		WillReturnError(errors.New("database error"))

	result, err := repo.GetPendingPDFLessons(context.Background(), 10)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPendingPDFLessons_ScanError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "pdf",
		"test-lesson", "Test Lesson", true, 1, "https://example.com/lesson.pdf", "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	).RowError(0, errors.New("scan error"))

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN \(
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  \)
		ORDER BY "createdAt" ASC
		LIMIT \$1`).
		WithArgs(10).
		WillReturnRows(rows)

	result, err := repo.GetPendingPDFLessons(context.Background(), 10)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPendingPDFLessons_RowsError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "pdf",
		"test-lesson", "Test Lesson", true, 1, "https://example.com/lesson.pdf", "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	).CloseError(errors.New("rows error"))

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN \(
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  \)
		ORDER BY "createdAt" ASC
		LIMIT \$1`).
		WithArgs(10).
		WillReturnRows(rows)

	result, err := repo.GetPendingPDFLessons(context.Background(), 10)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPendingPDFLessons_NullMediaURL(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "createdAt", "updatedAt", "access", "referenceAccess", "type", "slug", "name",
		"published", "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries",
		"thumbnail", "content", "moduleId", "createdBy", "showDescriptionToggle",
		"bannersTitle", "transcriptionCompleted",
	}).AddRow(
		"lesson-123", time.Now(), time.Now(), 1, "public", "pdf",
		"test-lesson", "Test Lesson", true, 1, nil, "completed",
		"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg",
		"Test content", "module-123", "user-123", true, "Test Banner", true,
	)

	mockDB.ExpectQuery(`SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN \(
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  \)
		ORDER BY "createdAt" ASC
		LIMIT \$1`).
		WithArgs(10).
		WillReturnRows(rows)

	result, err := repo.GetPendingPDFLessons(context.Background(), 10)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result))
	assert.Nil(t, result[0].MediaURL)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_Update_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	lessonID := "lesson-123"
	mediaURL := "https://example.com/media.mp4"
	lesson := &entities.Lesson{
		ID:                     &lessonID,
		Access:                 intPtr(1),
		ReferenceAccess:        stringPtr("public"),
		Type:                   stringPtr("video"),
		Slug:                   stringPtr("test-lesson"),
		Name:                   stringPtr("Test Lesson"),
		Published:              true,
		Order:                  intPtr(1),
		MediaURL:               &mediaURL,
		FullHDStatus:           stringPtr("completed"),
		FullHDURL:              stringPtr("https://example.com/video.mp4"),
		FullHDRetries:          intPtr(0),
		Thumbnail:              stringPtr("https://example.com/thumb.jpg"),
		Content:                stringPtr("Test content"),
		ModuleID:               stringPtr("module-123"),
		CreatedBy:              stringPtr("user-123"),
		ShowDescriptionToggle:  true,
		BannersTitle:           stringPtr("Test Banner"),
		TranscriptionCompleted: true,
	}

	mockDB.ExpectExec(`UPDATE "Lesson" 
		SET "updatedAt" = \$1, access = \$2, "referenceAccess" = \$3, type = \$4, slug = \$5, 
		    name = \$6, published = \$7, "order" = \$8, "mediaUrl" = \$9, "fullHdStatus" = \$10, 
		    "fullHdUrl" = \$11, "fullHdRetries" = \$12, thumbnail = \$13, content = \$14, 
		    "moduleId" = \$15, "createdBy" = \$16, "showDescriptionToggle" = \$17, 
		    "bannersTitle" = \$18, "transcriptionCompleted" = \$19
		WHERE id = \$20`).
		WithArgs(sqlmock.AnyArg(), 1, "public", "video", "test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
			"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg", "Test content", "module-123", "user-123", true, "Test Banner", true, "lesson-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Update(context.Background(), lesson)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_Update_WithNullMediaURL(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	lessonID := "lesson-123"
	lesson := &entities.Lesson{
		ID:                     &lessonID,
		Access:                 intPtr(1),
		ReferenceAccess:        stringPtr("public"),
		Type:                   stringPtr("video"),
		Slug:                   stringPtr("test-lesson"),
		Name:                   stringPtr("Test Lesson"),
		Published:              true,
		Order:                  intPtr(1),
		MediaURL:               nil,
		FullHDStatus:           stringPtr("completed"),
		FullHDURL:              stringPtr("https://example.com/video.mp4"),
		FullHDRetries:          intPtr(0),
		Thumbnail:              stringPtr("https://example.com/thumb.jpg"),
		Content:                stringPtr("Test content"),
		ModuleID:               stringPtr("module-123"),
		CreatedBy:              stringPtr("user-123"),
		ShowDescriptionToggle:  true,
		BannersTitle:           stringPtr("Test Banner"),
		TranscriptionCompleted: true,
	}

	mockDB.ExpectExec(`UPDATE "Lesson" 
		SET "updatedAt" = \$1, access = \$2, "referenceAccess" = \$3, type = \$4, slug = \$5, 
		    name = \$6, published = \$7, "order" = \$8, "mediaUrl" = \$9, "fullHdStatus" = \$10, 
		    "fullHdUrl" = \$11, "fullHdRetries" = \$12, thumbnail = \$13, content = \$14, 
		    "moduleId" = \$15, "createdBy" = \$16, "showDescriptionToggle" = \$17, 
		    "bannersTitle" = \$18, "transcriptionCompleted" = \$19
		WHERE id = \$20`).
		WithArgs(sqlmock.AnyArg(), 1, "public", "video", "test-lesson", "Test Lesson", true, 1, nil, "completed",
			"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg", "Test content", "module-123", "user-123", true, "Test Banner", true, "lesson-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Update(context.Background(), lesson)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_Update_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	lessonID := "lesson-123"
	mediaURL := "https://example.com/media.mp4"
	lesson := &entities.Lesson{
		ID:                     &lessonID,
		Access:                 intPtr(1),
		ReferenceAccess:        stringPtr("public"),
		Type:                   stringPtr("video"),
		Slug:                   stringPtr("test-lesson"),
		Name:                   stringPtr("Test Lesson"),
		Published:              true,
		Order:                  intPtr(1),
		MediaURL:               &mediaURL,
		FullHDStatus:           stringPtr("completed"),
		FullHDURL:              stringPtr("https://example.com/video.mp4"),
		FullHDRetries:          intPtr(0),
		Thumbnail:              stringPtr("https://example.com/thumb.jpg"),
		Content:                stringPtr("Test content"),
		ModuleID:               stringPtr("module-123"),
		CreatedBy:              stringPtr("user-123"),
		ShowDescriptionToggle:  true,
		BannersTitle:           stringPtr("Test Banner"),
		TranscriptionCompleted: true,
	}

	mockDB.ExpectExec(`UPDATE "Lesson" 
		SET "updatedAt" = \$1, access = \$2, "referenceAccess" = \$3, type = \$4, slug = \$5, 
		    name = \$6, published = \$7, "order" = \$8, "mediaUrl" = \$9, "fullHdStatus" = \$10, 
		    "fullHdUrl" = \$11, "fullHdRetries" = \$12, thumbnail = \$13, content = \$14, 
		    "moduleId" = \$15, "createdBy" = \$16, "showDescriptionToggle" = \$17, 
		    "bannersTitle" = \$18, "transcriptionCompleted" = \$19
		WHERE id = \$20`).
		WithArgs(sqlmock.AnyArg(), 1, "public", "video", "test-lesson", "Test Lesson", true, 1, mediaURL, "completed",
			"https://example.com/video.mp4", 0, "https://example.com/thumb.jpg", "Test content", "module-123", "user-123", true, "Test Banner", true, "lesson-123").
		WillReturnError(errors.New("database error"))

	err := repo.Update(context.Background(), lesson)

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFAssetByLessonID_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE "lessonId" = \$1`).
		WithArgs("lesson-123").
		WillReturnError(errors.New("database error"))

	result, err := repo.GetPDFAssetByLessonID(context.Background(), "lesson-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFAssetByLessonID_WithNullValues(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "lessonId", "sourcePdfUrl", "totalPages", "status", "error", "createdAt", "updatedAt",
	}).AddRow(
		"asset-123", "lesson-123", "https://example.com/source.pdf", nil, "processing", nil,
		time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE "lessonId" = \$1`).
		WithArgs("lesson-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFAssetByLessonID(context.Background(), "lesson-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "asset-123", result.ID)
	assert.Nil(t, result.TotalPages)
	assert.Nil(t, result.Error)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFAsset_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 5
	asset := &entities.LessonPDFAsset{
		ID:           "asset-123",
		LessonID:     "lesson-123",
		SourcePDFURL: "https://example.com/source.pdf",
		TotalPages:   &totalPages,
		Status:       "processing",
		Error:        nil,
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfAsset" \(id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("asset-123", "lesson-123", "https://example.com/source.pdf", 5, "processing", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreatePDFAsset(context.Background(), asset)

	assert.NoError(t, err)
	assert.NotZero(t, asset.CreatedAt)
	assert.NotZero(t, asset.UpdatedAt)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFAsset_WithNullValues(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	errorMsg := "test error"
	asset := &entities.LessonPDFAsset{
		ID:           "asset-123",
		LessonID:     "lesson-123",
		SourcePDFURL: "https://example.com/source.pdf",
		TotalPages:   nil,
		Status:       "failed",
		Error:        &errorMsg,
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfAsset" \(id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("asset-123", "lesson-123", "https://example.com/source.pdf", nil, "failed", "test error", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreatePDFAsset(context.Background(), asset)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFAsset_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 5
	asset := &entities.LessonPDFAsset{
		ID:           "asset-123",
		LessonID:     "lesson-123",
		SourcePDFURL: "https://example.com/source.pdf",
		TotalPages:   &totalPages,
		Status:       "processing",
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfAsset" \(id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("asset-123", "lesson-123", "https://example.com/source.pdf", 5, "processing", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("database error"))

	err := repo.CreatePDFAsset(context.Background(), asset)

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_UpdatePDFAsset_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 10
	asset := &entities.LessonPDFAsset{
		ID:           "asset-123",
		SourcePDFURL: "https://example.com/source.pdf",
		TotalPages:   &totalPages,
		Status:       "completed",
		Error:        nil,
	}

	mockDB.ExpectExec(`UPDATE "LessonPdfAsset" 
		SET "sourcePdfUrl" = \$1, "totalPages" = \$2, status = \$3, error = \$4, "updatedAt" = \$5
		WHERE id = \$6`).
		WithArgs("https://example.com/source.pdf", 10, "completed", nil, sqlmock.AnyArg(), "asset-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpdatePDFAsset(context.Background(), asset)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_UpdatePDFAsset_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 10
	asset := &entities.LessonPDFAsset{
		ID:           "asset-123",
		SourcePDFURL: "https://example.com/source.pdf",
		TotalPages:   &totalPages,
		Status:       "completed",
	}

	mockDB.ExpectExec(`UPDATE "LessonPdfAsset" 
		SET "sourcePdfUrl" = \$1, "totalPages" = \$2, status = \$3, error = \$4, "updatedAt" = \$5
		WHERE id = \$6`).
		WithArgs("https://example.com/source.pdf", 10, "completed", nil, sqlmock.AnyArg(), "asset-123").
		WillReturnError(errors.New("database error"))

	err := repo.UpdatePDFAsset(context.Background(), asset)

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_UpdatePDFAssetStatus_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 5
	status := "completed"

	mockDB.ExpectExec(`UPDATE "LessonPdfAsset" 
		SET status = \$1, "totalPages" = \$2, error = \$3, "updatedAt" = \$4
		WHERE id = \$5`).
		WithArgs("completed", 5, nil, sqlmock.AnyArg(), "asset-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpdatePDFAssetStatus(context.Background(), "asset-123", status, &totalPages, nil)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_UpdatePDFAssetStatus_WithNullValues(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	errorMsg := "processing error"
	status := "failed"

	mockDB.ExpectExec(`UPDATE "LessonPdfAsset" 
		SET status = \$1, "totalPages" = \$2, error = \$3, "updatedAt" = \$4
		WHERE id = \$5`).
		WithArgs("failed", nil, "processing error", sqlmock.AnyArg(), "asset-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpdatePDFAssetStatus(context.Background(), "asset-123", status, nil, &errorMsg)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_UpdatePDFAssetStatus_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	totalPages := 5
	status := "completed"

	mockDB.ExpectExec(`UPDATE "LessonPdfAsset" 
		SET status = \$1, "totalPages" = \$2, error = \$3, "updatedAt" = \$4
		WHERE id = \$5`).
		WithArgs("completed", 5, nil, sqlmock.AnyArg(), "asset-123").
		WillReturnError(errors.New("database error"))

	err := repo.UpdatePDFAssetStatus(context.Background(), "asset-123", status, &totalPages, nil)

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetFailedPDFAssets_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "lessonId", "sourcePdfUrl", "totalPages", "status", "error", "createdAt", "updatedAt",
	}).AddRow(
		"asset-123", "lesson-123", "https://example.com/source.pdf", 5, "failed", "error message",
		time.Now(), time.Now(),
	).AddRow(
		"asset-456", "lesson-456", "https://example.com/source2.pdf", nil, "partial", nil,
		time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE status IN \('failed', 'partial'\)
		ORDER BY "updatedAt" ASC`).
		WillReturnRows(rows)

	result, err := repo.GetFailedPDFAssets(context.Background())

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "asset-123", result[0].ID)
	assert.Equal(t, "failed", result[0].Status)
	assert.Equal(t, "asset-456", result[1].ID)
	assert.Equal(t, "partial", result[1].Status)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetFailedPDFAssets_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE status IN \('failed', 'partial'\)
		ORDER BY "updatedAt" ASC`).
		WillReturnError(errors.New("database error"))

	result, err := repo.GetFailedPDFAssets(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetFailedPDFAssets_ScanError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "lessonId", "sourcePdfUrl", "totalPages", "status", "error", "createdAt", "updatedAt",
	}).AddRow(
		"asset-123", "lesson-123", "https://example.com/source.pdf", 5, "failed", "error message",
		time.Now(), time.Now(),
	).RowError(0, errors.New("scan error"))

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE status IN \('failed', 'partial'\)
		ORDER BY "updatedAt" ASC`).
		WillReturnRows(rows)

	result, err := repo.GetFailedPDFAssets(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetFailedPDFAssets_RowsError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "lessonId", "sourcePdfUrl", "totalPages", "status", "error", "createdAt", "updatedAt",
	}).AddRow(
		"asset-123", "lesson-123", "https://example.com/source.pdf", 5, "failed", "error message",
		time.Now(), time.Now(),
	).CloseError(errors.New("rows error"))

	mockDB.ExpectQuery(`SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE status IN \('failed', 'partial'\)
		ORDER BY "updatedAt" ASC`).
		WillReturnRows(rows)

	result, err := repo.GetFailedPDFAssets(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFPage_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	width := 1920
	height := 1080
	page := &entities.LessonPDFPage{
		ID:         "page-123",
		AssetID:    "asset-123",
		PageNumber: 1,
		ImageURL:   "https://example.com/page1.jpg",
		Width:      &width,
		Height:     &height,
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfPage" \(id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("page-123", "asset-123", 1, "https://example.com/page1.jpg", 1920, 1080, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreatePDFPage(context.Background(), page)

	assert.NoError(t, err)
	assert.NotZero(t, page.CreatedAt)
	assert.NotZero(t, page.UpdatedAt)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFPage_WithNullDimensions(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	page := &entities.LessonPDFPage{
		ID:         "page-123",
		AssetID:    "asset-123",
		PageNumber: 1,
		ImageURL:   "https://example.com/page1.jpg",
		Width:      nil,
		Height:     nil,
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfPage" \(id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("page-123", "asset-123", 1, "https://example.com/page1.jpg", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreatePDFPage(context.Background(), page)

	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_CreatePDFPage_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	page := &entities.LessonPDFPage{
		ID:         "page-123",
		AssetID:    "asset-123",
		PageNumber: 1,
		ImageURL:   "https://example.com/page1.jpg",
	}

	mockDB.ExpectExec(`INSERT INTO "LessonPdfPage" \(id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\)`).
		WithArgs("page-123", "asset-123", 1, "https://example.com/page1.jpg", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("database error"))

	err := repo.CreatePDFPage(context.Background(), page)

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPageByAssetAndNumber_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-123", "asset-123", 1, "https://example.com/page1.jpg", 1920, 1080, time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1 AND "pageNumber" = \$2`).
		WithArgs("asset-123", 1).
		WillReturnRows(rows)

	result, err := repo.GetPDFPageByAssetAndNumber(context.Background(), "asset-123", 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "page-123", result.ID)
	assert.Equal(t, 1, result.PageNumber)
	assert.Equal(t, 1920, *result.Width)
	assert.Equal(t, 1080, *result.Height)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPageByAssetAndNumber_NotFound(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1 AND "pageNumber" = \$2`).
		WithArgs("asset-123", 1).
		WillReturnError(sql.ErrNoRows)

	result, err := repo.GetPDFPageByAssetAndNumber(context.Background(), "asset-123", 1)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrPDFPageNotFound, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPageByAssetAndNumber_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1 AND "pageNumber" = \$2`).
		WithArgs("asset-123", 1).
		WillReturnError(errors.New("database error"))

	result, err := repo.GetPDFPageByAssetAndNumber(context.Background(), "asset-123", 1)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPageByAssetAndNumber_WithNullDimensions(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-123", "asset-123", 1, "https://example.com/page1.jpg", nil, nil, time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1 AND "pageNumber" = \$2`).
		WithArgs("asset-123", 1).
		WillReturnRows(rows)

	result, err := repo.GetPDFPageByAssetAndNumber(context.Background(), "asset-123", 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Nil(t, result.Width)
	assert.Nil(t, result.Height)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_Success(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-1", "asset-123", 1, "https://example.com/page1.jpg", 1920, 1080, time.Now(), time.Now(),
	).AddRow(
		"page-2", "asset-123", 2, "https://example.com/page2.jpg", 1920, 1080, time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "page-1", result[0].ID)
	assert.Equal(t, 1, result[0].PageNumber)
	assert.Equal(t, "page-2", result[1].ID)
	assert.Equal(t, 2, result[1].PageNumber)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_Empty(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	})

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.NoError(t, err)
	if result != nil {
		assert.Equal(t, 0, len(result))
	} else {
		assert.Nil(t, result)
	}
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnError(errors.New("database error"))

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_ScanError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-1", "asset-123", 1, "https://example.com/page1.jpg", 1920, 1080, time.Now(), time.Now(),
	).RowError(0, errors.New("scan error"))

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_RowsError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-1", "asset-123", 1, "https://example.com/page1.jpg", 1920, 1080, time.Now(), time.Now(),
	).CloseError(errors.New("rows error"))

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_GetPDFPagesByAssetID_WithNullDimensions(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	repo := &LessonRepository{db: db, log: mockLogger}

	rows := sqlmock.NewRows([]string{
		"id", "assetId", "pageNumber", "imageUrl", "width", "height", "createdAt", "updatedAt",
	}).AddRow(
		"page-1", "asset-123", 1, "https://example.com/page1.jpg", nil, nil, time.Now(), time.Now(),
	)

	mockDB.ExpectQuery(`SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = \$1
		ORDER BY "pageNumber" ASC`).
		WithArgs("asset-123").
		WillReturnRows(rows)

	result, err := repo.GetPDFPagesByAssetID(context.Background(), "asset-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result))
	assert.Nil(t, result[0].Width)
	assert.Nil(t, result[0].Height)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_DeletePDFPage_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectExec(`DELETE FROM "LessonPdfPage" WHERE id = \$1`).
		WithArgs("page-123").
		WillReturnError(errors.New("database error"))

	err := repo.DeletePDFPage(context.Background(), "page-123")

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func TestLessonRepository_DeletePDFPagesByAssetID_DatabaseError(t *testing.T) {
	db, mockDB, _ := sqlmock.New()
	mockLogger := &mocks.MockLogger{}

	mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()

	repo := &LessonRepository{db: db, log: mockLogger}

	mockDB.ExpectExec(`DELETE FROM "LessonPdfPage" WHERE "assetId" = \$1`).
		WithArgs("asset-123").
		WillReturnError(errors.New("database error"))

	err := repo.DeletePDFPagesByAssetID(context.Background(), "asset-123")

	assert.Error(t, err)
	assert.IsType(t, &memberclasserrors.MemberClassError{}, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
}

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
