package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
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

func (m *MockCommentRepository) FindAllByTenant(ctx context.Context, tenantID string, req *request.GetCommentsRequest) ([]*dto.CommentResponse, int64, error) {
	args := m.Called(ctx, tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.CommentResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockCommentRepository) FindUserByEmailAndTenant(ctx context.Context, email, tenantID string) (string, error) {
	args := m.Called(ctx, email, tenantID)
	return args.String(0), args.Error(1)
}

func TestNewCommentUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	assert.NotNil(t, useCase)
}

func TestCommentUseCase_UpdateAnswer_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

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

	req := request.UpdateCommentRequest{
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
	assert.NotNil(t, result.Answer)
	assert.Equal(t, answer, *result.Answer)
	assert.NotNil(t, result.Published)
	assert.Equal(t, published, *result.Published)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_UpdateAnswer_EmptyAnswer(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	commentID := "comment-123"
	tenantID := "tenant-123"
	answer := "Updated answer"
	question := "Test question?"

	existingComment := &entities.Comment{
		ID:       &commentID,
		Question: &question,
	}

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

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

	req := request.UpdateCommentRequest{
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
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

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

	publishedFalse := false
	response := &dto.CommentResponse{
		ID:         commentID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Question:   "Test question?",
		Answer:     &answer,
		Published:  &publishedFalse,
		LessonName: "Lesson 1",
		CourseName: "Course 1",
		Username:   "User 1",
		UserEmail:  "user1@test.com",
	}

	req := request.UpdateCommentRequest{
		Answer:    answer,
		Published: nil,
	}

	mockRepo.On("FindByIDAndTenant", mock.Anything, commentID, tenantID).Return(existingComment, nil)
	mockRepo.On("Update", mock.Anything, commentID, answer, false).Return(updatedComment, nil)
	mockRepo.On("FindByIDAndTenantWithDetails", mock.Anything, commentID, tenantID).Return(response, nil)

	result, err := useCase.UpdateAnswer(context.Background(), commentID, tenantID, req)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Published)
	assert.Equal(t, false, *result.Published)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_Success(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	tenantID := "tenant-123"
	req := &request.GetCommentsRequest{
		Page:  1,
		Limit: 10,
	}

	answer1 := "Answer 1"
	answer2 := "Answer 2"
	published1 := true
	published2 := false
	comments := []*dto.CommentResponse{
		{
			ID:         "comment-1",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Question:   "Question 1?",
			Answer:     &answer1,
			Published:  &published1,
			LessonName: "Lesson 1",
			CourseName: "Course 1",
			Username:   "User 1",
			UserEmail:  "user1@test.com",
		},
		{
			ID:         "comment-2",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Question:   "Question 2?",
			Answer:     &answer2,
			Published:  &published2,
			LessonName: "Lesson 2",
			CourseName: "Course 2",
			Username:   "User 2",
			UserEmail:  "user2@test.com",
		},
	}

	total := int64(2)

	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, req).Return(comments, total, nil)

	result, err := useCase.GetComments(context.Background(), tenantID, req)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Comments))
	assert.Equal(t, int64(2), result.Pagination.TotalCount)
	assert.Equal(t, 1, result.Pagination.Page)
	assert.Equal(t, 10, result.Pagination.Limit)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	tenantID := "tenant-123"
	req := &request.GetCommentsRequest{
		Page:  1,
		Limit: 10,
	}

	repoError := errors.New("database error")
	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, req).Return(nil, int64(0), repoError)

	result, err := useCase.GetComments(context.Background(), tenantID, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repoError, err)

	mockRepo.AssertExpectations(t)
}

func TestCommentUseCase_GetComments_EmptyResult(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := new(MockCommentRepository)
	mockUserRepo := mocks.NewMockUserRepository(t)

	useCase := NewCommentUseCase(mockLogger, mockRepo, mockUserRepo)

	tenantID := "tenant-123"
	req := &request.GetCommentsRequest{
		Page:  1,
		Limit: 10,
	}

	comments := []*dto.CommentResponse{}
	total := int64(0)

	mockRepo.On("FindAllByTenant", mock.Anything, tenantID, req).Return(comments, total, nil)

	result, err := useCase.GetComments(context.Background(), tenantID, req)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Comments))
	assert.Equal(t, int64(0), result.Pagination.TotalCount)

	mockRepo.AssertExpectations(t)
}
