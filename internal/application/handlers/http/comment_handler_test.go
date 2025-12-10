package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockCommentUseCase struct {
	mock.Mock
}

func (m *MockCommentUseCase) UpdateAnswer(ctx context.Context, commentID, tenantID string, req dto.UpdateCommentRequest) (*dto.CommentResponse, error) {
	args := m.Called(ctx, commentID, tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CommentResponse), args.Error(1)
}

func (m *MockCommentUseCase) GetComments(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) (*dto.CommentsPaginationResponse, error) {
	args := m.Called(ctx, tenantID, pagination)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CommentsPaginationResponse), args.Error(1)
}

func TestNewCommentHandler(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
	assert.NotNil(t, handler.paginationUtils)
}

func TestCommentHandler_GetComments_Success(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	comments := []*dto.CommentResponse{
		{
			ID:         "comment-1",
			Question:   "Test question?",
			Answer:     "Test answer",
			Published:  true,
			UpdatedAt:  time.Now(),
			LessonName: "Lesson 1",
			CourseName: "Course 1",
			UserName:   "User 1",
			UserEmail:  "user1@test.com",
		},
	}

	response := &dto.CommentsPaginationResponse{
		Data: []dto.CommentResponse{
			*comments[0],
		},
		Pagination: dto.PaginationMeta{
			Page:       1,
			PageSize:   10,
			Total:      1,
			TotalPages: 1,
			HasNext:    false,
			HasPrev:    false,
		},
	}

	mockUseCase.On("GetComments", mock.Anything, tenantID, mock.Anything).Return(response, nil)

	req := httptest.NewRequest("GET", "/api/comments", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result dto.CommentsPaginationResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Data))
	assert.Equal(t, "comment-1", result.Data[0].ID)

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_GetComments_MethodNotAllowed(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", response.Error)
}

func TestCommentHandler_GetComments_TenantNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Tenant not found in context", response.Message)
}

func TestCommentHandler_GetComments_UseCaseError(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    http.StatusInternalServerError,
		Message: "Database error",
	}

	mockUseCase.On("GetComments", mock.Anything, tenantID, mock.Anything).Return(nil, memberClassErr)

	req := httptest.NewRequest("GET", "/api/comments", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Database error", response.Message)

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_Success(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := dto.UpdateCommentRequest{
		Answer:    "Updated answer",
		Published: boolPtr(true),
	}

	response := &dto.CommentResponse{
		ID:         commentID,
		Question:   "Test question?",
		Answer:     "Updated answer",
		Published:  true,
		UpdatedAt:  time.Now(),
		LessonName: "Lesson 1",
		CourseName: "Course 1",
		UserName:   "User 1",
		UserEmail:  "user1@test.com",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(response, nil)

	reqBody := `{"answer": "Updated answer", "published": true}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result dto.CommentResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, commentID, result.ID)
	assert.Equal(t, "Updated answer", result.Answer)

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_MethodNotAllowed(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/comments/comment-123", nil)
	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", response.Error)
}

func TestCommentHandler_UpdateComment_MissingCommentID(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("PATCH", "/api/comments/", nil)
	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Comment ID is required", response.Message)
}

func TestCommentHandler_UpdateComment_TenantNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Tenant not found in context", response.Message)
}

func TestCommentHandler_UpdateComment_InvalidBody(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	reqBody := `{"invalid": json}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Invalid request body", response.Message)
}

func TestCommentHandler_UpdateComment_CommentNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := dto.UpdateCommentRequest{
		Answer: "Updated answer",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, usecases.ErrCommentNotFound)

	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Comment not found", response.Message)

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_AnswerRequired(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := dto.UpdateCommentRequest{
		Answer: "",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, usecases.ErrAnswerRequired)

	reqBody := `{"answer": ""}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response.Message, "answer")

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_UnexpectedError(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := dto.UpdateCommentRequest{
		Answer: "Updated answer",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Internal server error", response.Message)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func boolPtr(b bool) *bool {
	return &b
}
