package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockCommentRepository struct {
	mock.Mock
}

func (m *MockCommentRepository) Update(ctx context.Context, commentID, answer string, published bool) (*entities.Comment, error) {
	args := m.Called(ctx, commentID, answer, published)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.Comment), args.Error(1)
}

func (m *MockCommentRepository) FindByIDAndTenant(ctx context.Context, commentID, tenantID string) (*entities.Comment, error) {
	args := m.Called(ctx, commentID, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.Comment), args.Error(1)
}

func (m *MockCommentRepository) FindByIDAndTenantWithDetails(ctx context.Context, commentID, tenantID string) (*dto.CommentResponse, error) {
	args := m.Called(ctx, commentID, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CommentResponse), args.Error(1)
}

func (m *MockCommentRepository) FindAllByTenant(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) ([]*dto.CommentResponse, int64, error) {
	args := m.Called(ctx, tenantID, pagination)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.CommentResponse), args.Get(1).(int64), args.Error(2)
}

func TestNewCommentUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	assert.NotNil(t, useCase)
}

func TestCommentUseCase_UpdateAnswer_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"
	published := true

	question := "Test question?"
	existingComment := &entities.Comment{
		ID:       &commentID,
		Question: &question,
		Answer:   nil,
	}

	updatedComment := &entities.Comment{
		ID:        &commentID,
		Question:  &question,
		Answer:    &answer,
		Published: published,
		UpdatedAt: time.Now(),
	}

	response := &dto.CommentResponse{
		ID:         commentID,
		Question:   "Test question?",
		Answer:     answer,
		Published:  published,
		UpdatedAt:  time.Now(),
		LessonName: "Lesson 1",
		CourseName: "Course 1",
		UserName:   "User 1",
		UserEmail:  "user1@test.com",
	}

	req := dto.UpdateCommentRequest{
		Answer:    answer,
		Published: &published,
	}

	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(existingComment, nil)
	mockRepo.On("Update", mock.Anything, commentID, answer, published).Return(updatedComment, nil)
	mockRepo.On("FindByIDAndTenantWithDetails", mock.Anything, commentID, tenantID).Return(response, nil)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, commentID, result.ID)
	assert.Equal(t, answer, result.Answer)
	assert.Equal(t, published, result.Published)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_EmptyAnswer(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"

	req := dto.UpdateCommentRequest{
		Answer: "",
	}

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrAnswerRequired, err)
}

func TestCommentUseCase_UpdateAnswer_CommentNotFound(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := dto.UpdateCommentRequest{
		Answer: answer,
	}

	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(nil, memberclasserrors.ErrCommentNotFound)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, memberclasserrors.ErrCommentNotFound, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_CommentNil(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := dto.UpdateCommentRequest{
		Answer: answer,
	}

	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(nil, nil)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrCommentNotFound, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_RepositoryErrorOnFind(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := dto.UpdateCommentRequest{
		Answer: answer,
	}

	repoError := errors.New("database error")
	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(nil, repoError)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_RepositoryErrorOnUpdate(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"
	question := "Test question?"

	existingComment := &entities.Comment{
		ID:       &commentID,
		Question: &question,
	}

	req := dto.UpdateCommentRequest{
		Answer: answer,
	}

	repoError := errors.New("update error")
	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(existingComment, nil)
	mockRepo.On("Update", mock.Anything, commentID, answer, false).Return(nil, repoError)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_RepositoryErrorOnFindDetails(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"
	question := "Test question?"

	existingComment := &entities.Comment{
		ID:       &commentID,
		Question: &question,
	}

	updatedComment := &entities.Comment{
		ID:        &commentID,
		Question:  &question,
		Answer:    &answer,
		Published: false,
		UpdatedAt: time.Now(),
	}

	req := dto.UpdateCommentRequest{
		Answer: answer,
	}

	repoError := errors.New("find details error")
	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(existingComment, nil)
	mockRepo.On("Update", mock.Anything, commentID, answer, false).Return(updatedComment, nil)
	mockRepo.On("FindByIDAndTenantWithDetails", mock.Anything, commentID, tenantID).Return(nil, repoError)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_PublishedNil(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	question := "Test question?"
	existingComment := &entities.Comment{
		ID:       &commentID,
		Question: &question,
	}

	updatedComment := &entities.Comment{
		ID:        &commentID,
		Question:  &question,
		Answer:    &answer,
		Published: false,
		UpdatedAt: time.Now(),
	}

	response := &dto.CommentResponse{
		ID:         commentID,
		Question:   "Test question?",
		Answer:     answer,
		Published:  false,
		UpdatedAt:  time.Now(),
		LessonName: "Lesson 1",
		CourseName: "Course 1",
		UserName:   "User 1",
		UserEmail:  "user1@test.com",
	}

	req := dto.UpdateCommentRequest{
		Answer:    answer,
		Published: nil,
	}

	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(existingComment, nil)
	mockRepo.On("Update", mock.Anything, commentID, answer, false).Return(updatedComment, nil)
	mockRepo.On("FindByIDAndTenantWithDetails", mock.Anything, commentID, tenantID).Return(response, nil)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, false, result.Published)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	tenantID := "tenant-123"
	pagination := &dto.PaginationRequest{
		Page:     1,
		PageSize: 10,
		SortBy:   "updatedAt",
		SortDir:  "desc",
	}

	comments := []*dto.CommentResponse{
		{
			ID:         "comment-1",
			Question:   "Question 1?",
			Answer:     "Answer 1",
			Published:  true,
			UpdatedAt:  time.Now(),
			LessonName: "Lesson 1",
			CourseName: "Course 1",
			UserName:   "User 1",
			UserEmail:  "user1@test.com",
		},
		{
			ID:         "comment-2",
			Question:   "Question 2?",
			Answer:     "Answer 2",
			Published:  false,
			UpdatedAt:  time.Now(),
			LessonName: "Lesson 2",
			CourseName: "Course 2",
			UserName:   "User 2",
			UserEmail:  "user2@test.com",
		},
	}

	total := int64(2)

	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, pagination).Return(comments, total, nil)

	result, err := useCase.GetComments(context.Background(), tenantID, pagination)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Data))
	assert.Equal(t, int64(2), result.Pagination.Total)
	assert.Equal(t, 1, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.PageSize)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	tenantID := "tenant-123"
	pagination := &dto.PaginationRequest{
		Page:     1,
		PageSize: 10,
		SortBy:   "updatedAt",
		SortDir:  "desc",
	}

	repoError := errors.New("database error")
	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, pagination).Return(nil, int64(0), repoError)

	result, err := useCase.GetComments(context.Background(), tenantID, pagination)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_EmptyResult(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)

	useCase := NewCommentUseCase(mockLogger, mockRepo)

	tenantID := "tenant-123"
	pagination := &dto.PaginationRequest{
		Page:     1,
		PageSize: 10,
		SortBy:   "updatedAt",
		SortDir:  "desc",
	}

	comments := []*dto.CommentResponse{}
	total := int64(0)

	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, pagination).Return(comments, total, nil)

	result, err := useCase.GetComments(context.Background(), tenantID, pagination)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Data))
	assert.Equal(t, int64(0), result.Pagination.Total)

	mockRepo.AssertExpectations(t)
}

