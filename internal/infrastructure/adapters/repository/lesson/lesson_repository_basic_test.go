package lesson

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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
	assert.Equal(t, "lesson-123", result.ID)
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
