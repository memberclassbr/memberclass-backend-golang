package comment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/social"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/usecases/comment"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewSocialCommentHandler(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestSocialCommentHandler_CreateOrUpdatePost_Success_Create(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	responseData := &social.SocialCommentResponse{
		OK: true,
		ID: "post-789",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(responseData, nil)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result social.SocialCommentResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.True(t, result.OK)
	assert.Equal(t, "post-789", result.ID)

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_Success_Update(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-789",
		Title:   "Updated Post",
		Content: "Updated Content",
	}

	responseData := &social.SocialCommentResponse{
		OK: true,
		ID: "post-789",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(responseData, nil)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result social.SocialCommentResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.True(t, result.OK)

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/social", nil)
	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Method Not Allowed", result.Error)
}

func TestSocialCommentHandler_CreateOrUpdatePost_InvalidBody(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBufferString("invalid json"))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Bad Request", result.Error)
}

func TestSocialCommentHandler_CreateOrUpdatePost_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "INVALID_API_KEY", result["errorCode"])
}

func TestSocialCommentHandler_CreateOrUpdatePost_UserNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, comment.ErrUserNotFoundOrNotInTenantForPost)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "PERMISSION_DENIED", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_PostNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-999",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, comment.ErrPostNotFound)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "POST_NOT_FOUND", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_TopicNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-999",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, comment.ErrTopicNotFound)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "TOPIC_NOT_FOUND", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MissingUser(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, errors.New("userId é obrigatório"))

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "MISSING_USER", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MissingTopic(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, errors.New("topicId é obrigatório para criar post"))

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "MISSING_TOPIC", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MissingTitle(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, errors.New("title é obrigatório"))

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "MISSING_TITLE", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MissingContent(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, errors.New("content é obrigatório"))

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, "MISSING_CONTENT", result["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestSocialCommentHandler_CreateOrUpdatePost_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockSocialCommentUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewSocialCommentHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reqBody := comments.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    500,
		Message: "database error",
	}

	mockUseCase.On("CreateOrUpdatePost", mock.Anything, reqBody, tenantID).Return(nil, memberClassErr)

	body, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/api/v1/social", bytes.NewBuffer(body))
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.CreateOrUpdatePost(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var result dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "Internal Server Error", result.Error)
	assert.Equal(t, "database error", result.Message)

	mockUseCase.AssertExpectations(t)
}

func stringPtr(s string) *string {
	return &s
}
