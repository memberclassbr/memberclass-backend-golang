package usecases

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/stretchr/testify/assert"
)

type mockLessonRepository struct {
	lessons           map[string]*entities.Lesson
	pdfAssets         map[string]*entities.LessonPDFAsset
	pdfPages          map[string][]*entities.LessonPDFPage
	createPageCalls   int
	updateStatusCalls int
	mu                sync.RWMutex
}

func (m *mockLessonRepository) GetLessonsWithHierarchyByTenant(ctx context.Context, tenantID string, onlyUnprocessed bool) ([]ports.AILessonWithHierarchy, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockLessonRepository) GetByIDWithTenant(ctx context.Context, lessonID string) (*entities.Lesson, *entities.Tenant, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockLessonRepository) UpdateTranscriptionStatus(ctx context.Context, lessonID string, transcriptionCompleted bool) error {
	//TODO implement me
	panic("implement me")
}

func newMockLessonRepository() *mockLessonRepository {
	return &mockLessonRepository{
		lessons:   make(map[string]*entities.Lesson),
		pdfAssets: make(map[string]*entities.LessonPDFAsset),
		pdfPages:  make(map[string][]*entities.LessonPDFPage),
	}
}

func (m *mockLessonRepository) GetByID(ctx context.Context, id string) (*entities.Lesson, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lesson, exists := m.lessons[id]
	if !exists {
		return nil, memberclasserrors.ErrLessonNotFound
	}
	return lesson, nil
}

func (m *mockLessonRepository) GetByIDWithPDFAsset(ctx context.Context, id string) (*entities.Lesson, error) {
	return m.GetByID(ctx, id)
}

func (m *mockLessonRepository) GetPendingPDFLessons(ctx context.Context, limit int) ([]*entities.Lesson, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var pending []*entities.Lesson
	for _, lesson := range m.lessons {
		if lesson.MediaURL != nil && *lesson.MediaURL != "" {
			pending = append(pending, lesson)
		}
	}

	if limit > 0 && len(pending) > limit {
		pending = pending[:limit]
	}

	return pending, nil
}

func (m *mockLessonRepository) Update(ctx context.Context, lesson *entities.Lesson) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lessons[*lesson.ID] = lesson
	return nil
}

func (m *mockLessonRepository) GetPDFAssetByLessonID(ctx context.Context, lessonID string) (*entities.LessonPDFAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, asset := range m.pdfAssets {
		if asset.LessonID == lessonID {
			return asset, nil
		}
	}
	return nil, memberclasserrors.ErrPDFAssetNotFound
}

func (m *mockLessonRepository) CreatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pdfAssets[asset.ID] = asset
	return nil
}

func (m *mockLessonRepository) UpdatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pdfAssets[asset.ID] = asset
	return nil
}

func (m *mockLessonRepository) UpdatePDFAssetStatus(ctx context.Context, assetID string, status string, totalPages *int, errorMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if asset, exists := m.pdfAssets[assetID]; exists {
		asset.Status = status
		if totalPages != nil {
			asset.TotalPages = totalPages
		}
		if errorMsg != nil {
			asset.Error = errorMsg
		}
		m.updateStatusCalls++
	}
	return nil
}

func (m *mockLessonRepository) GetFailedPDFAssets(ctx context.Context) ([]*entities.LessonPDFAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var failed []*entities.LessonPDFAsset
	for _, asset := range m.pdfAssets {
		if asset.Status == "failed" {
			failed = append(failed, asset)
		}
	}
	return failed, nil
}

func (m *mockLessonRepository) GetPDFAssetByID(ctx context.Context, assetID string) (*entities.LessonPDFAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	asset, exists := m.pdfAssets[assetID]
	if !exists {
		return nil, memberclasserrors.ErrPDFAssetNotFound
	}
	return asset, nil
}

func (m *mockLessonRepository) GetPDFPageByAssetAndNumber(ctx context.Context, assetID string, pageNumber int) (*entities.LessonPDFPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pages, exists := m.pdfPages[assetID]
	if !exists {
		return nil, memberclasserrors.ErrPDFPageNotFound
	}

	for _, page := range pages {
		if page.PageNumber == pageNumber {
			return page, nil
		}
	}
	return nil, memberclasserrors.ErrPDFPageNotFound
}

func (m *mockLessonRepository) CreatePDFPage(ctx context.Context, page *entities.LessonPDFPage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pdfPages[page.AssetID] = append(m.pdfPages[page.AssetID], page)
	m.createPageCalls++
	return nil
}

func (m *mockLessonRepository) GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*entities.LessonPDFPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pages, exists := m.pdfPages[assetID]
	if !exists {
		return nil, memberclasserrors.ErrPDFPageNotFound
	}
	return pages, nil
}

func (m *mockLessonRepository) DeletePDFPage(ctx context.Context, pageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for assetID, pages := range m.pdfPages {
		for i, page := range pages {
			if page.ID == pageID {
				m.pdfPages[assetID] = append(pages[:i], pages[i+1:]...)
				return nil
			}
		}
	}
	return memberclasserrors.ErrPDFPageNotFound
}

func (m *mockLessonRepository) DeletePDFPagesByAssetID(ctx context.Context, assetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pdfPages, assetID)
	return nil
}

func (m *mockLessonRepository) FindCompletedLessonsByEmail(ctx context.Context, userID, tenantID string, startDate, endDate time.Time, courseID string, page, limit int) ([]response.CompletedLesson, int64, error) {
	return []response.CompletedLesson{}, int64(0), nil
}

type mockPdfService struct {
	images []string
}

func newMockPdfService() *mockPdfService {
	return &mockPdfService{
		images: []string{"image1", "image2", "image3", "image4", "image5"},
	}
}

func (m *mockPdfService) GetToken() (string, error) {
	return "mock-token", nil
}

func (m *mockPdfService) CreateTask(token string) (*dto.TaskResponse, error) {
	return &dto.TaskResponse{
		Task:   "mock-task",
		Server: "mock-server",
	}, nil
}

func (m *mockPdfService) AddFile(token, taskID, pdfURL, server string) (string, error) {
	return "mock-filename", nil
}

func (m *mockPdfService) ProcessTask(token, taskID, serverFilename, server string) error {
	return nil
}

func (m *mockPdfService) DownloadTask(token, taskID, server string) ([]byte, error) {
	return []byte("mock-zip-data"), nil
}

func (m *mockPdfService) ExtractImagesFromZip(zipData []byte) ([]string, error) {
	return m.images, nil
}

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, args ...any) {}
func (m *mockLogger) Info(msg string, args ...any)  {}
func (m *mockLogger) Warn(msg string, args ...any)  {}
func (m *mockLogger) Error(msg string, args ...any) {}

type mockStorageService struct{}

func (m *mockStorageService) Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error) {
	return "https://storage.example.com/" + filename, nil
}

func (m *mockStorageService) Download(ctx context.Context, urlOrKey string) ([]byte, error) {
	return []byte("mock-data"), nil
}

func (m *mockStorageService) Delete(ctx context.Context, urlOrKey string) error {
	return nil
}

func (m *mockStorageService) Exists(ctx context.Context, urlOrKey string) (bool, error) {
	return true, nil
}

func stringPtr(s string) *string {
	return &s
}

func TestNewPdfProcessorUseCase(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := NewPdfProcessorUseCase(repo, pdfService, storageService, logger)

	assert.NotNil(t, useCase)
}

func TestProcessAllPendingLessons_NoLessons(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	result, err := useCase.ProcessAllPendingLessons(ctx, 10)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Results)
}

func TestProcessAllPendingLessons_WithLessons(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	lessons := []*entities.Lesson{
		{ID: stringPtr("lesson1"), MediaURL: stringPtr("http://example.com/doc1.pdf")},
		{ID: stringPtr("lesson2"), MediaURL: stringPtr("http://example.com/doc2.pdf")},
	}

	for _, lesson := range lessons {
		repo.lessons[*lesson.ID] = lesson
	}

	ctx := context.Background()
	result, err := useCase.ProcessAllPendingLessons(ctx, 10)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Results, 2)
}

func TestProcessLesson_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	lessonID := "lesson1"
	lesson := &entities.Lesson{
		ID:       &lessonID,
		MediaURL: stringPtr("http://example.com/doc1.pdf"),
	}
	repo.lessons[lessonID] = lesson

	ctx := context.Background()
	result, err := useCase.ProcessLesson(ctx, lessonID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestProcessLesson_NoMediaURL(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	lessonID := "lesson1"
	lesson := &entities.Lesson{
		ID: &lessonID,
	}
	repo.lessons[lessonID] = lesson

	ctx := context.Background()
	result, err := useCase.ProcessLesson(ctx, lessonID)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestProcessLesson_LessonNotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	result, err := useCase.ProcessLesson(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestSaveSinglePage_NewPage(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	assetID := "test-asset"
	pageNumber := 1
	imageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

	created, err := useCase.saveSinglePage(ctx, assetID, pageNumber, imageBase64)

	assert.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, 1, repo.createPageCalls)
}

func TestSaveSinglePage_ExistingPage(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	existingPage := &entities.LessonPDFPage{
		AssetID:    "test-asset",
		PageNumber: 1,
		ImageURL:   "existing-url",
	}
	repo.pdfPages["test-asset"] = []*entities.LessonPDFPage{existingPage}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	assetID := "test-asset"
	pageNumber := 1
	imageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

	created, err := useCase.saveSinglePage(ctx, assetID, pageNumber, imageBase64)

	assert.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, 0, repo.createPageCalls)
}

func TestSaveSinglePage_InvalidBase64(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	assetID := "test-asset"
	pageNumber := 1
	imageBase64 := "invalid-base64"

	created, err := useCase.saveSinglePage(ctx, assetID, pageNumber, imageBase64)

	assert.Error(t, err)
	assert.False(t, created)
	assert.Equal(t, 0, repo.createPageCalls)
}

func TestConvertPdfToImages_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	pdfURL := "http://example.com/test.pdf"
	images, err := useCase.ConvertPdfToImages(pdfURL)

	assert.NoError(t, err)
	assert.Len(t, images, 5)
}

func TestCreateOrUpdatePDFAsset_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	lessonID := "lesson1"
	pdfURL := "http://example.com/test.pdf"

	asset, err := useCase.CreateOrUpdatePDFAsset(context.Background(), lessonID, pdfURL)

	assert.NoError(t, err)
	assert.NotNil(t, asset)
	assert.Equal(t, lessonID, asset.LessonID)
	assert.Equal(t, pdfURL, asset.SourcePDFURL)
}

func TestValidateLessonHasPDF_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID:       stringPtr("lesson1"),
		MediaURL: stringPtr("http://example.com/test.pdf"),
		PDFAsset: &entities.LessonPDFAsset{
			ID:       "asset1",
			LessonID: "lesson1",
			Status:   "completed",
		},
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.ValidateLessonHasPDF(context.Background(), "lesson1")

	assert.NoError(t, err)
}

func TestValidateLessonHasPDF_NoPDF(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID: stringPtr("lesson1"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.ValidateLessonHasPDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestGetLessonWithPDFAsset_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID: stringPtr("lesson1"),
		PDFAsset: &entities.LessonPDFAsset{
			ID:       "asset1",
			LessonID: "lesson1",
			Status:   "completed",
		},
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.GetLessonWithPDFAsset(context.Background(), "lesson1")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.ID)
	assert.Equal(t, "lesson1", *result.ID)
	assert.NotNil(t, result.PDFAsset)
}

func TestGetLessonWithPDFAsset_NotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.GetLessonWithPDFAsset(context.Background(), "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestRetryFailedAssets_NoFailedAssets(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RetryFailedAssets(context.Background())

	assert.NoError(t, err)
}

func TestCleanupOrphanedPages_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.CleanupOrphanedPages(context.Background())

	assert.NoError(t, err)
}

func TestRegeneratePDF_Success(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID:       stringPtr("lesson1"),
		MediaURL: stringPtr("http://example.com/test.pdf"),
		PDFAsset: &entities.LessonPDFAsset{
			ID:       "asset1",
			LessonID: "lesson1",
			Status:   "completed",
		},
	}
	repo.lessons["lesson1"] = lesson
	repo.pdfAssets["asset1"] = lesson.PDFAsset

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.NoError(t, err)
}

func TestRetryFailedAssets_WithFailedAssets(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	failedAsset := &entities.LessonPDFAsset{
		ID:       "asset1",
		LessonID: "lesson1",
		Status:   "failed",
	}
	repo.pdfAssets["asset1"] = failedAsset

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RetryFailedAssets(context.Background())

	assert.NoError(t, err)
}

func TestCleanupOrphanedPages_WithOrphanedPages(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	orphanedPage := &entities.LessonPDFPage{
		ID:         "page1",
		AssetID:    "nonexistent-asset",
		PageNumber: 1,
		ImageURL:   "http://example.com/page1.jpg",
	}
	repo.pdfPages["nonexistent-asset"] = []*entities.LessonPDFPage{orphanedPage}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.CleanupOrphanedPages(context.Background())

	assert.NoError(t, err)
}

func TestProcessLesson_PdfServiceError(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := &mockPdfServiceWithError{}
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lessonID := "lesson1"
	lesson := &entities.Lesson{
		ID:       &lessonID,
		MediaURL: stringPtr("http://example.com/doc1.pdf"),
	}
	repo.lessons[lessonID] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	result, err := useCase.ProcessLesson(ctx, lessonID)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestConvertPdfToImages_PdfServiceError(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := &mockPdfServiceWithError{}
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	pdfURL := "http://example.com/test.pdf"
	images, err := useCase.ConvertPdfToImages(pdfURL)

	assert.Error(t, err)
	assert.Nil(t, images)
}

func TestCreateOrUpdatePDFAsset_ExistingAsset(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	existingAsset := &entities.LessonPDFAsset{
		ID:           "asset1",
		LessonID:     "lesson1",
		SourcePDFURL: "http://example.com/old.pdf",
		Status:       "processing",
	}
	repo.pdfAssets["asset1"] = existingAsset

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	lessonID := "lesson1"
	pdfURL := "http://example.com/new.pdf"

	asset, err := useCase.CreateOrUpdatePDFAsset(context.Background(), lessonID, pdfURL)

	assert.NoError(t, err)
	assert.NotNil(t, asset)
	assert.Equal(t, "http://example.com/old.pdf", asset.SourcePDFURL)
}

func TestProcessAllPendingLessons_WithLimit(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lessons := []*entities.Lesson{
		{ID: stringPtr("lesson1"), MediaURL: stringPtr("http://example.com/doc1.pdf")},
		{ID: stringPtr("lesson2"), MediaURL: stringPtr("http://example.com/doc2.pdf")},
		{ID: stringPtr("lesson3"), MediaURL: stringPtr("http://example.com/doc3.pdf")},
		{ID: stringPtr("lesson4"), MediaURL: stringPtr("http://example.com/doc4.pdf")},
		{ID: stringPtr("lesson5"), MediaURL: stringPtr("http://example.com/doc5.pdf")},
	}

	for _, lesson := range lessons {
		repo.lessons[*lesson.ID] = lesson
	}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	result, err := useCase.ProcessAllPendingLessons(ctx, 3)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Results, 3)
	assert.Equal(t, 3, result.Total)
}

type mockPdfServiceWithError struct{}

func (m *mockPdfServiceWithError) GetToken() (string, error) {
	return "", assert.AnError
}

func (m *mockPdfServiceWithError) CreateTask(token string) (*dto.TaskResponse, error) {
	return nil, assert.AnError
}

func (m *mockPdfServiceWithError) AddFile(token, taskID, pdfURL, server string) (string, error) {
	return "", assert.AnError
}

func (m *mockPdfServiceWithError) ProcessTask(token, taskID, serverFilename, server string) error {
	return assert.AnError
}

func (m *mockPdfServiceWithError) DownloadTask(token, taskID, server string) ([]byte, error) {
	return nil, assert.AnError
}

func (m *mockPdfServiceWithError) ExtractImagesFromZip(zipData []byte) ([]string, error) {
	return nil, assert.AnError
}

func TestRegeneratePDF_LessonNotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "nonexistent")

	assert.Error(t, err)
}

func TestRegeneratePDF_NoMediaURL(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID: stringPtr("lesson1"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestRegeneratePDF_NoPDFAsset(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID:       stringPtr("lesson1"),
		MediaURL: stringPtr("http://example.com/test.pdf"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.NoError(t, err)
}

func TestValidateLessonHasPDF_LessonNotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.ValidateLessonHasPDF(context.Background(), "nonexistent")

	assert.Error(t, err)
}

func TestValidateLessonHasPDF_NoMediaURL(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID: stringPtr("lesson1"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.ValidateLessonHasPDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestGetLessonWithPDFAsset_LessonNotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.GetLessonWithPDFAsset(context.Background(), "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCreateOrUpdatePDFAsset_LessonNotFound(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	asset, err := useCase.CreateOrUpdatePDFAsset(context.Background(), "nonexistent", "http://example.com/test.pdf")

	assert.NoError(t, err)
	assert.NotNil(t, asset)
}

func TestCreateOrUpdatePDFAsset_NoMediaURL(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID: stringPtr("lesson1"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	asset, err := useCase.CreateOrUpdatePDFAsset(context.Background(), "lesson1", "http://example.com/test.pdf")

	assert.NoError(t, err)
	assert.NotNil(t, asset)
}

func TestSaveSinglePage_StorageServiceError(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageServiceWithError{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	assetID := "test-asset"
	pageNumber := 1
	imageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

	created, err := useCase.saveSinglePage(ctx, assetID, pageNumber, imageBase64)

	assert.Error(t, err)
	assert.False(t, created)
	assert.Equal(t, 0, repo.createPageCalls)
}

type mockStorageServiceWithError struct{}

func (m *mockStorageServiceWithError) Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error) {
	return "", assert.AnError
}

func (m *mockStorageServiceWithError) Download(ctx context.Context, urlOrKey string) ([]byte, error) {
	return nil, assert.AnError
}

func (m *mockStorageServiceWithError) Delete(ctx context.Context, urlOrKey string) error {
	return assert.AnError
}

func (m *mockStorageServiceWithError) Exists(ctx context.Context, urlOrKey string) (bool, error) {
	return false, assert.AnError
}

func TestCleanupOrphanedPages_WithFailedAssets(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	failedAsset := &entities.LessonPDFAsset{
		ID:       "asset1",
		LessonID: "lesson1",
		Status:   "failed",
	}
	repo.pdfAssets["asset1"] = failedAsset

	pages := []*entities.LessonPDFPage{
		{ID: "page1", AssetID: "asset1", PageNumber: 1, ImageURL: "http://example.com/page1.jpg"},
		{ID: "page2", AssetID: "asset1", PageNumber: 2, ImageURL: "http://example.com/page2.jpg"},
	}
	repo.pdfPages["asset1"] = pages

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.CleanupOrphanedPages(context.Background())

	assert.NoError(t, err)
}

func TestCleanupOrphanedPages_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.CleanupOrphanedPages(context.Background())

	assert.Error(t, err)
}

func TestRetryFailedAssets_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RetryFailedAssets(context.Background())

	assert.Error(t, err)
}

func TestProcessAllPendingLessons_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.ProcessAllPendingLessons(context.Background(), 10)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestProcessLesson_CreateAssetError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.ProcessLesson(context.Background(), "lesson1")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestProcessLesson_SavePagesError(t *testing.T) {
	repo := newMockLessonRepository()
	pdfService := newMockPdfService()
	storageService := &mockStorageServiceWithError{}
	logger := &mockLogger{}

	lesson := &entities.Lesson{
		ID:       stringPtr("lesson1"),
		MediaURL: stringPtr("http://example.com/doc1.pdf"),
	}
	repo.lessons["lesson1"] = lesson

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.ProcessLesson(context.Background(), "lesson1")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, 0, result.ProcessedPages)
}

func TestProcessLesson_UpdateStatusError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.ProcessLesson(context.Background(), "lesson1")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestRegeneratePDF_DeletePagesError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestRegeneratePDF_UpdateAssetError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestRegeneratePDF_CreateAssetError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.RegeneratePDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestValidateLessonHasPDF_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	err := useCase.ValidateLessonHasPDF(context.Background(), "lesson1")

	assert.Error(t, err)
}

func TestGetLessonWithPDFAsset_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	result, err := useCase.GetLessonWithPDFAsset(context.Background(), "lesson1")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCreateOrUpdatePDFAsset_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	asset, err := useCase.CreateOrUpdatePDFAsset(context.Background(), "lesson1", "http://example.com/test.pdf")

	assert.Error(t, err)
	assert.Nil(t, asset)
}

func TestSaveSinglePage_RepositoryError(t *testing.T) {
	repo := &mockLessonRepositoryWithError{}
	pdfService := newMockPdfService()
	storageService := &mockStorageService{}
	logger := &mockLogger{}

	useCase := &pdfProcessorUseCase{
		lessonRepo:     repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}

	ctx := context.Background()
	assetID := "test-asset"
	pageNumber := 1
	imageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

	created, err := useCase.saveSinglePage(ctx, assetID, pageNumber, imageBase64)

	assert.Error(t, err)
	assert.False(t, created)
}

type mockLessonRepositoryWithError struct{}

func (m *mockLessonRepositoryWithError) GetLessonsWithHierarchyByTenant(ctx context.Context, tenantID string, onlyUnprocessed bool) ([]ports.AILessonWithHierarchy, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockLessonRepositoryWithError) GetByIDWithTenant(ctx context.Context, lessonID string) (*entities.Lesson, *entities.Tenant, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockLessonRepositoryWithError) UpdateTranscriptionStatus(ctx context.Context, lessonID string, transcriptionCompleted bool) error {
	//TODO implement me
	panic("implement me")
}

func (m *mockLessonRepositoryWithError) GetByID(ctx context.Context, id string) (*entities.Lesson, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) GetByIDWithPDFAsset(ctx context.Context, id string) (*entities.Lesson, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) GetPendingPDFLessons(ctx context.Context, limit int) ([]*entities.Lesson, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) Update(ctx context.Context, lesson *entities.Lesson) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) GetPDFAssetByLessonID(ctx context.Context, lessonID string) (*entities.LessonPDFAsset, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) CreatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) UpdatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) UpdatePDFAssetStatus(ctx context.Context, assetID string, status string, totalPages *int, errorMsg *string) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) GetFailedPDFAssets(ctx context.Context) ([]*entities.LessonPDFAsset, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) GetPDFAssetByID(ctx context.Context, assetID string) (*entities.LessonPDFAsset, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) GetPDFPageByAssetAndNumber(ctx context.Context, assetID string, pageNumber int) (*entities.LessonPDFPage, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) CreatePDFPage(ctx context.Context, page *entities.LessonPDFPage) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*entities.LessonPDFPage, error) {
	return nil, assert.AnError
}

func (m *mockLessonRepositoryWithError) DeletePDFPage(ctx context.Context, pageID string) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) DeletePDFPagesByAssetID(ctx context.Context, assetID string) error {
	return assert.AnError
}

func (m *mockLessonRepositoryWithError) FindCompletedLessonsByEmail(ctx context.Context, userID, tenantID string, startDate, endDate time.Time, courseID string, page, limit int) ([]response.CompletedLesson, int64, error) {
	return nil, int64(0), assert.AnError
}
