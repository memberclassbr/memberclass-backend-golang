package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewSocialCommentUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	assert.NotNil(t, useCase)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID: "",
		Title:  "Test",
	}

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "userId")
}

func TestSocialCommentUseCase_CreateOrUpdatePost_UserNotInTenant(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test",
		Content: "Content",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(false, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrUserNotFoundOrNotInTenantForPost, err)

	mockUserRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_CreatePost_Success_Owner(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	topic := &ports.TopicInfo{
		ID:          "topic-456",
		OnlyAdmin:   false,
		DeliveryIDs: []string{},
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(true, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-456").Return(topic, nil)
	mockSocialCommentRepo.On("Create", mock.Anything, req, "tenant-123").Return("post-789", nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, "post-789", result.ID)

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_CreatePost_TopicNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-999",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-999").Return(nil, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrTopicNotFound, err)

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_CreatePost_OnlyAdmin_NotOwner(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	topic := &ports.TopicInfo{
		ID:          "topic-456",
		OnlyAdmin:   true,
		DeliveryIDs: []string{},
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-456").Return(topic, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrNoAccessToTopic, err)

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_CreatePost_NoDeliveryAccess(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	topic := &ports.TopicInfo{
		ID:          "topic-456",
		OnlyAdmin:   false,
		DeliveryIDs: []string{"delivery-1", "delivery-2"},
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-456").Return(topic, nil)
	mockUserRepo.On("GetUserDeliveryIDs", mock.Anything, "user-123").Return([]string{"delivery-3"}, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrNoAccessToTopic, err)

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_CreatePost_WithDeliveryAccess(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	topic := &ports.TopicInfo{
		ID:          "topic-456",
		OnlyAdmin:   false,
		DeliveryIDs: []string{"delivery-1", "delivery-2"},
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-456").Return(topic, nil)
	mockUserRepo.On("GetUserDeliveryIDs", mock.Anything, "user-123").Return([]string{"delivery-1", "delivery-3"}, nil)
	mockSocialCommentRepo.On("Create", mock.Anything, req, "tenant-123").Return("post-789", nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, "post-789", result.ID)

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_UpdatePost_Success_Owner(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-789",
		Title:   "Updated Post",
		Content: "Updated Content",
	}

	post := &ports.PostInfo{
		ID:     "post-789",
		UserID: "user-456",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockSocialCommentRepo.On("FindByID", mock.Anything, "post-789").Return(post, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(true, nil)
	mockSocialCommentRepo.On("Update", mock.Anything, req, "tenant-123").Return(nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, "post-789", result.ID)

	mockUserRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_UpdatePost_Success_OwnPost(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-789",
		Title:   "Updated Post",
		Content: "Updated Content",
	}

	post := &ports.PostInfo{
		ID:     "post-789",
		UserID: "user-123",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockSocialCommentRepo.On("FindByID", mock.Anything, "post-789").Return(post, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)
	mockSocialCommentRepo.On("Update", mock.Anything, req, "tenant-123").Return(nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.OK)

	mockUserRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_UpdatePost_PostNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-999",
		Title:   "Updated Post",
		Content: "Updated Content",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockSocialCommentRepo.On("FindByID", mock.Anything, "post-999").Return(nil, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrPostNotFound, err)

	mockUserRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_UpdatePost_PermissionDenied(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		PostID:  "post-789",
		Title:   "Updated Post",
		Content: "Updated Content",
	}

	post := &ports.PostInfo{
		ID:     "post-789",
		UserID: "user-456",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockSocialCommentRepo.On("FindByID", mock.Anything, "post-789").Return(post, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(false, nil)

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrPermissionDenied, err)

	mockUserRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

func TestSocialCommentUseCase_CreateOrUpdatePost_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	mockSocialCommentRepo := mocks.NewMockSocialCommentRepository(t)
	mockTopicRepo := mocks.NewMockTopicRepository(t)

	useCase := NewSocialCommentUseCase(mockLogger, mockUserRepo, mockSocialCommentRepo, mockTopicRepo)

	req := request.CreateSocialCommentRequest{
		UserID:  "user-123",
		TopicID: "topic-456",
		Title:   "Test Post",
		Content: "Test Content",
	}

	mockUserRepo.On("BelongsToTenant", "user-123", "tenant-123").Return(true, nil)
	mockUserRepo.On("IsUserOwner", mock.Anything, "user-123", "tenant-123").Return(true, nil)
	mockTopicRepo.On("FindByIDWithDeliveries", mock.Anything, "topic-456").Return(&ports.TopicInfo{ID: "topic-456"}, nil)
	mockSocialCommentRepo.On("Create", mock.Anything, req, "tenant-123").Return("", errors.New("database error"))

	result, err := useCase.CreateOrUpdatePost(context.Background(), req, "tenant-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "database error", err.Error())

	mockUserRepo.AssertExpectations(t)
	mockTopicRepo.AssertExpectations(t)
	mockSocialCommentRepo.AssertExpectations(t)
}

