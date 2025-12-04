package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations
type MockPdfProcessorUseCase struct {
	mock.Mock
}

func (m *MockPdfProcessorUseCase) ProcessLesson(ctx context.Context, lessonID string) (*dto.ProcessResult, error) {
	args := m.Called(ctx, lessonID)
	return args.Get(0).(*dto.ProcessResult), args.Error(1)
}

func (m *MockPdfProcessorUseCase) ProcessAllPendingLessons(ctx context.Context, limit int) (*dto.BatchProcessResult, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).(*dto.BatchProcessResult), args.Error(1)
}

func (m *MockPdfProcessorUseCase) RetryFailedAssets(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPdfProcessorUseCase) CleanupOrphanedPages(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPdfProcessorUseCase) RegeneratePDF(ctx context.Context, lessonID string) error {
	args := m.Called(ctx, lessonID)
	return args.Error(0)
}

func (m *MockPdfProcessorUseCase) ConvertPdfToImages(pdfURL string) ([]string, error) {
	args := m.Called(pdfURL)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockPdfProcessorUseCase) CreateOrUpdatePDFAsset(ctx context.Context, lessonID, pdfURL string) (*entities.LessonPDFAsset, error) {
	args := m.Called(ctx, lessonID, pdfURL)
	return args.Get(0).(*entities.LessonPDFAsset), args.Error(1)
}

func (m *MockPdfProcessorUseCase) SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
	args := m.Called(ctx, assetID, lessonID, images)
	return args.Int(0), args.Error(1)
}

func (m *MockPdfProcessorUseCase) ValidateLessonHasPDF(ctx context.Context, lessonID string) error {
	args := m.Called(ctx, lessonID)
	return args.Error(0)
}

func (m *MockPdfProcessorUseCase) GetLessonWithPDFAsset(ctx context.Context, lessonID string) (*entities.Lesson, error) {
	args := m.Called(ctx, lessonID)
	return args.Get(0).(*entities.Lesson), args.Error(1)
}

func (m *MockPdfProcessorUseCase) GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*entities.LessonPDFPage, error) {
	args := m.Called(ctx, assetID)
	return args.Get(0).([]*entities.LessonPDFPage), args.Error(1)
}

type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Error(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Debug(msg string, args ...any) {
	m.Called(msg, args)
}

// Test cases
func TestGetLessonsPage_Success(t *testing.T) {
	// Setup
	mockUseCase := new(MockPdfProcessorUseCase)
	mockLogger := new(MockLogger)

	handler := &LessonHandler{
		useCase:         mockUseCase,
		logger:          mockLogger,
		paginationUtils: usecases.NewPaginationUtils(),
	}

	lessonID := "test-lesson-001"
	assetID := "test-asset-001"

	// Mock lesson with PDF asset
	lesson := &entities.Lesson{
		ID: &lessonID,
		PDFAsset: &entities.LessonPDFAsset{
			ID:         assetID,
			LessonID:   lessonID,
			Status:     "done",
			TotalPages: intPtr(3),
		},
	}

	// Mock PDF pages
	pages := []*entities.LessonPDFPage{
		{
			ID:         "page1",
			AssetID:    assetID,
			PageNumber: 1,
			ImageURL:   "https://memberclass.sfo3.digitaloceanspaces.com/lessons/test-lesson-001/pdf-pages/page-1.jpg",
			Width:      nil,
			Height:     nil,
		},
		{
			ID:         "page2",
			AssetID:    assetID,
			PageNumber: 2,
			ImageURL:   "https://memberclass.sfo3.digitaloceanspaces.com/lessons/test-lesson-001/pdf-pages/page-2.jpg",
			Width:      nil,
			Height:     nil,
		},
		{
			ID:         "page3",
			AssetID:    assetID,
			PageNumber: 3,
			ImageURL:   "https://memberclass.sfo3.digitaloceanspaces.com/lessons/test-lesson-001/pdf-pages/page-3.jpg",
			Width:      nil,
			Height:     nil,
		},
	}

	// Setup expectations
	mockUseCase.On("GetLessonWithPDFAsset", mock.Anything, lessonID).Return(lesson, nil)
	mockUseCase.On("GetPDFPagesByAssetID", mock.Anything, assetID).Return(pages, nil)

	// Create request
	req := httptest.NewRequest("GET", "/api/lessons/"+lessonID+"/pdf-pages", nil)

	// Add lessonId to URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("lessonId", lessonID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	// Execute
	handler.GetLessonsPage(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.LessonPDFPagesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lessonID, response.LessonID)
	assert.Equal(t, "done", response.Status)
	assert.Equal(t, 3, response.TotalPages)
	assert.Len(t, response.Pages, 3)

	// Verify page details
	assert.Equal(t, 1, response.Pages[0].PageNumber)
	assert.Equal(t, "https://memberclass.sfo3.digitaloceanspaces.com/lessons/test-lesson-001/pdf-pages/page-1.jpg", response.Pages[0].ImageURL)
	assert.Nil(t, response.Pages[0].Width)
	assert.Nil(t, response.Pages[0].Height)

	mockUseCase.AssertExpectations(t)
}

func TestGetLessonsPage_NoAsset(t *testing.T) {
	// Setup
	mockUseCase := new(MockPdfProcessorUseCase)
	mockLogger := new(MockLogger)

	handler := &LessonHandler{
		useCase:         mockUseCase,
		logger:          mockLogger,
		paginationUtils: usecases.NewPaginationUtils(),
	}

	lessonID := "test-lesson-001"

	// Mock lesson without PDF asset
	lesson := &entities.Lesson{
		ID:       &lessonID,
		PDFAsset: nil,
	}

	// Setup expectations
	mockUseCase.On("GetLessonWithPDFAsset", mock.Anything, lessonID).Return(lesson, nil)

	// Create request
	req := httptest.NewRequest("GET", "/api/lessons/"+lessonID+"/pdf-pages", nil)

	// Add lessonId to URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("lessonId", lessonID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	// Execute
	handler.GetLessonsPage(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.LessonPDFPagesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, lessonID, response.LessonID)
	assert.Equal(t, "absent", response.Status)
	assert.Equal(t, 0, response.TotalPages)
	assert.Len(t, response.Pages, 0)

	mockUseCase.AssertExpectations(t)
}

func TestGetLessonsPage_MissingLessonId(t *testing.T) {
	// Setup
	mockUseCase := new(MockPdfProcessorUseCase)
	mockLogger := new(MockLogger)

	handler := &LessonHandler{
		useCase:         mockUseCase,
		logger:          mockLogger,
		paginationUtils: usecases.NewPaginationUtils(),
	}

	// Create request without lessonId
	req := httptest.NewRequest("GET", "/api/lessons//pdf-pages", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.GetLessonsPage(w, req)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "lessonId is required", response.Message)
}

func TestGetLessonsPage_LessonNotFound(t *testing.T) {
	// Setup
	mockUseCase := new(MockPdfProcessorUseCase)
	mockLogger := new(MockLogger)

	handler := &LessonHandler{
		useCase:         mockUseCase,
		logger:          mockLogger,
		paginationUtils: usecases.NewPaginationUtils(),
	}

	lessonID := "nonexistent-lesson"

	// Setup expectations
	mockUseCase.On("GetLessonWithPDFAsset", mock.Anything, lessonID).Return((*entities.Lesson)(nil), &memberclasserrors.MemberClassError{
		Code:    404,
		Message: "lesson not found",
	})
	mockLogger.On("Error", mock.AnythingOfType("string"), mock.Anything).Return()

	// Create request
	req := httptest.NewRequest("GET", "/api/lessons/"+lessonID+"/pdf-pages", nil)

	// Add lessonId to URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("lessonId", lessonID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	// Execute
	handler.GetLessonsPage(w, req)

	// Assertions
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Error fetching lesson", response.Message)

	mockUseCase.AssertExpectations(t)
}

// Helper function
func intPtr(i int) *int {
	return &i
}
