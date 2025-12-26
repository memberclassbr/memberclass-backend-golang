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
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
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

func (m *MockCommentUseCase) UpdateAnswer(ctx context.Context, commentID, tenantID string, req request.UpdateCommentRequest) (*dto.CommentResponse, error) {
	args := m.Called(ctx, commentID, tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CommentResponse), args.Error(1)
}

func (m *MockCommentUseCase) GetComments(ctx context.Context, tenantID string, req *request.GetCommentsRequest) (*dto.CommentsPaginationResponse, error) {
	args := m.Called(ctx, tenantID, req)
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
}

func TestCommentHandler_GetComments_Success(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	answer := "Test answer"
	published := true
	comments := []*dto.CommentResponse{
		{
			ID:         "comment-1",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Question:   "Test question?",
			Answer:     &answer,
			Published:  &published,
			LessonName: "Lesson 1",
			CourseName: "Course 1",
			Username:   "User 1",
			UserEmail:  "user1@test.com",
		},
	}

	response := &dto.CommentsPaginationResponse{
		Comments: []dto.CommentResponse{
			*comments[0],
		},
		Pagination: dto.CommentsPaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetComments", mock.Anything, tenantID, mock.Anything).Return(response, nil)

	req := httptest.NewRequest("GET", "/api/v1/comments?page=1&limit=10", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result dto.CommentsPaginationResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Comments))
	assert.Equal(t, "comment-1", result.Comments[0].ID)

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_GetComments_MethodNotAllowed(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Method not allowed", response["error"])
	assert.Equal(t, "METHOD_NOT_ALLOWED", response["errorCode"])
}

func TestCommentHandler_GetComments_TenantNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key inválida", response["error"])
	assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
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

	req := httptest.NewRequest("GET", "/api/v1/comments?page=1&limit=10", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Database error", response["error"])

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_Success(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := request.UpdateCommentRequest{
		Answer:    "Updated answer",
		Published: boolPtr(true),
	}

	answer := "Updated answer"
	published := true
	response := &dto.CommentResponse{
		ID:         commentID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Question:   "Test question?",
		Answer:     &answer,
		Published:  &published,
		LessonName: "Lesson 1",
		CourseName: "Course 1",
		Username:   "User 1",
		UserEmail:  "user1@test.com",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(response, nil)

	reqBody := `{"answer": "Updated answer", "published": true}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, true, result["ok"])
	
	commentData := result["comment"].(map[string]interface{})
	assert.Equal(t, commentID, commentData["id"])
	assert.Equal(t, "Updated answer", commentData["answer"])

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_MethodNotAllowed(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/comments/comment-123", nil)
	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Method not allowed", response["error"])
	assert.Equal(t, "METHOD_NOT_ALLOWED", response["errorCode"])
}

func TestCommentHandler_UpdateComment_MissingCommentID(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("PATCH", "/api/v1/comments/", nil)
	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Comment ID is required", response["error"])
	assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
}

func TestCommentHandler_UpdateComment_TenantNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key inválida", response["error"])
	assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
}

func TestCommentHandler_UpdateComment_InvalidBody(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	reqBody := `{"invalid": json}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Invalid request body", response["error"])
	assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
}

func TestCommentHandler_UpdateComment_CommentNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := request.UpdateCommentRequest{
		Answer: "Updated answer",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, usecases.ErrCommentNotFound)

	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Comentário não encontrado ou não pertence a este tenant", response["error"])
	assert.Equal(t, "COMMENT_NOT_FOUND", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_AnswerRequired(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := request.UpdateCommentRequest{
		Answer: "",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, usecases.ErrAnswerRequired)

	reqBody := `{"answer": ""}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Contains(t, response["error"], "answer")
	assert.Equal(t, "INVALID_REQUEST", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_UpdateComment_UnexpectedError(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	commentID := "comment-123"
	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	requestBody := request.UpdateCommentRequest{
		Answer: "Updated answer",
	}

	mockUseCase.On("UpdateAnswer", mock.Anything, commentID, tenantID, requestBody).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	reqBody := `{"answer": "Updated answer"}`
	req := httptest.NewRequest("PATCH", "/api/v1/comments/"+commentID, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentID", commentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.UpdateComment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Erro interno do servidor", response["error"])
	assert.Equal(t, "INTERNAL_ERROR", response["errorCode"])

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestCommentHandler_GetComments_InvalidPagination(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	req := httptest.NewRequest("GET", "/api/v1/comments?page=0&limit=10", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Contains(t, response["error"], "paginação")
	assert.Equal(t, "INVALID_PAGINATION", response["errorCode"])
}

func TestCommentHandler_GetComments_WithFilters(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	answer := "Test answer"
	published := true
	comments := []*dto.CommentResponse{
		{
			ID:         "comment-1",
			CreatedAt:  time.Now(),
			UpdatedAt:    time.Now(),
			Question:   "Test question?",
			Answer:     &answer,
			Published:  &published,
			LessonName: "Lesson 1",
			CourseName: "Course 1",
			Username:   "User 1",
			UserEmail:  "user1@test.com",
		},
	}

	response := &dto.CommentsPaginationResponse{
		Comments: []dto.CommentResponse{
			*comments[0],
		},
		Pagination: dto.CommentsPaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	mockUseCase.On("GetComments", mock.Anything, tenantID, mock.Anything).Return(response, nil)

	req := httptest.NewRequest("GET", "/api/v1/comments?page=1&limit=10&email=user@test.com&status=approved&courseId=course-123&answered=true", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result dto.CommentsPaginationResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Comments))

	mockUseCase.AssertExpectations(t)
}

func TestCommentHandler_GetComments_UserNotFound(t *testing.T) {
	mockUseCase := new(MockCommentUseCase)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &entities.Tenant{ID: tenantID}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    404,
		Message: "Usuário não encontrado",
	}

	mockUseCase.On("GetComments", mock.Anything, tenantID, mock.Anything).Return(nil, memberClassErr)

	req := httptest.NewRequest("GET", "/api/v1/comments?page=1&limit=10&email=notfound@test.com", nil)
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, tenant)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Usuário não encontrado", response["error"])
	assert.Equal(t, "USER_NOT_FOUND", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func boolPtr(b bool) *bool {
	return &b
}
