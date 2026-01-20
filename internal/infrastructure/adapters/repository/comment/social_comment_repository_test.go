package comment

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports/comment"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewSocialCommentRepository(t *testing.T) {
	t.Run("should create new social comment repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewSocialCommentRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}

func TestSocialCommentRepository_Create(t *testing.T) {
	image := "https://example.com/image.jpg"
	videoEmbed := "https://example.com/video.mp4"

	tests := []struct {
		name          string
		req           comments.CreateSocialCommentRequest
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedID    string
	}{
		{
			name: "should create post successfully",
			req: comments.CreateSocialCommentRequest{
				UserID:     "user-123",
				TopicID:    "topic-123",
				Title:      "Test Post",
				Content:    "Test Content",
				Image:      &image,
				VideoEmbed: &videoEmbed,
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`INSERT INTO "Post"`).
					WithArgs(sqlmock.AnyArg(), "topic-123", "Test Post", "Test Content", &image, &videoEmbed, "user-123").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("post-123"))
			},
			expectedError: nil,
			expectedID:    "post-123",
		},
		{
			name: "should create post without image and video",
			req: comments.CreateSocialCommentRequest{
				UserID:  "user-123",
				TopicID: "topic-123",
				Title:   "Test Post",
				Content: "Test Content",
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`INSERT INTO "Post"`).
					WithArgs(sqlmock.AnyArg(), "topic-123", "Test Post", "Test Content", nil, nil, "user-123").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("post-456"))
			},
			expectedError: nil,
			expectedID:    "post-456",
		},
		{
			name: "should return MemberClassError when database error occurs",
			req: comments.CreateSocialCommentRequest{
				UserID:  "user-123",
				TopicID: "topic-123",
				Title:   "Test Post",
				Content: "Test Content",
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`INSERT INTO "Post"`).
					WithArgs(sqlmock.AnyArg(), "topic-123", "Test Post", "Test Content", nil, nil, "user-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error creating post",
			},
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewSocialCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.Create(context.Background(), tt.req, tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Equal(t, tt.expectedID, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				if tt.expectedID != "" {
					assert.Equal(t, tt.expectedID, result)
				}
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestSocialCommentRepository_FindByID(t *testing.T) {
	tests := []struct {
		name          string
		postID        string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedPost  *comment.PostInfo
	}{
		{
			name:   "should return post when found",
			postID: "post-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "userId"}).
					AddRow("post-123", "user-123")
				sqlMock.ExpectQuery(`SELECT id, "userId"`).
					WithArgs("post-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedPost: &comment.PostInfo{
				ID:     "post-123",
				UserID: "user-123",
			},
		},
		{
			name:   "should return nil when post does not exist",
			postID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "userId"`).
					WithArgs("non-existent").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError: nil,
			expectedPost:  nil,
		},
		{
			name:   "should return MemberClassError when database error occurs",
			postID: "post-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT id, "userId"`).
					WithArgs("post-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding post",
			},
			expectedPost: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewSocialCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByID(context.Background(), tt.postID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectedPost == nil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
					assert.Equal(t, tt.expectedPost.ID, result.ID)
					assert.Equal(t, tt.expectedPost.UserID, result.UserID)
				}
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestSocialCommentRepository_Update(t *testing.T) {
	image := "https://example.com/image.jpg"
	videoEmbed := "https://example.com/video.mp4"

	tests := []struct {
		name          string
		req           comments.CreateSocialCommentRequest
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
	}{
		{
			name: "should update post successfully",
			req: comments.CreateSocialCommentRequest{
				PostID:     "post-123",
				Title:      "Updated Post",
				Content:    "Updated Content",
				Image:      &image,
				VideoEmbed: &videoEmbed,
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectExec(`UPDATE "Post"`).
					WithArgs("post-123", "Updated Post", "Updated Content", &image, &videoEmbed).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectedError: nil,
		},
		{
			name: "should update post without image and video",
			req: comments.CreateSocialCommentRequest{
				PostID:  "post-123",
				Title:   "Updated Post",
				Content: "Updated Content",
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectExec(`UPDATE "Post"`).
					WithArgs("post-123", "Updated Post", "Updated Content", nil, nil).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectedError: nil,
		},
		{
			name: "should return MemberClassError when database error occurs",
			req: comments.CreateSocialCommentRequest{
				PostID:  "post-123",
				Title:   "Updated Post",
				Content: "Updated Content",
			},
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectExec(`UPDATE "Post"`).
					WithArgs("post-123", "Updated Post", "Updated Content", nil, nil).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error updating post",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewSocialCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			err = repository.Update(context.Background(), tt.req, tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}
