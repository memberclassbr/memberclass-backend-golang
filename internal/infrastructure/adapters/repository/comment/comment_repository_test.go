package comment

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
)

func TestNewCommentRepository(t *testing.T) {
	t.Run("should create new comment repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewCommentRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}

func TestCommentRepository_FindByIDAndTenant(t *testing.T) {
	tests := []struct {
		name          string
		commentID     string
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedComment *entities.Comment
	}{
		{
			name:      "should return comment when found",
			commentID: "comment-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				question := "Test question?"
				answer := "Test answer"
				lessonID := "lesson-123"
				userID := "user-123"
				createdAt := time.Now()
				updatedAt := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "createdAt", "updatedAt", "lessonId", "userId",
				}).AddRow(
					"comment-123", question, answer, true, createdAt, updatedAt, lessonID, userID,
				)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("comment-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedComment: func() *entities.Comment {
				commentID := "comment-123"
				question := "Test question?"
				answer := "Test answer"
				lessonID := "lesson-123"
				userID := "user-123"
				return &entities.Comment{
					ID:        &commentID,
					Question:  &question,
					Answer:    &answer,
					Published: true,
					LessonID:  &lessonID,
					UserID:    &userID,
				}
			}(),
		},
		{
			name:      "should return comment with null answer and published",
			commentID: "comment-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				question := "Test question?"
				lessonID := "lesson-123"
				userID := "user-123"
				createdAt := time.Now()
				updatedAt := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "createdAt", "updatedAt", "lessonId", "userId",
				}).AddRow(
					"comment-123", question, nil, nil, createdAt, updatedAt, lessonID, userID,
				)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("comment-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedComment: func() *entities.Comment {
				commentID := "comment-123"
				question := "Test question?"
				lessonID := "lesson-123"
				userID := "user-123"
				return &entities.Comment{
					ID:        &commentID,
					Question:  &question,
					Answer:    nil,
					Published: false,
					LessonID:  &lessonID,
					UserID:    &userID,
				}
			}(),
		},
		{
			name:      "should return ErrCommentNotFound when comment does not exist",
			commentID: "non-existent",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("non-existent", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError:   memberclasserrors.ErrCommentNotFound,
			expectedComment: nil,
		},
		{
			name:      "should return error when database error occurs",
			commentID: "comment-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("comment-123", "tenant-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError:   errors.New("database connection error"),
			expectedComment: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByIDAndTenant(context.Background(), tt.commentID, tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedError, memberclasserrors.ErrCommentNotFound) {
					assert.Equal(t, memberclasserrors.ErrCommentNotFound, err)
				} else {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expectedComment.ID, *result.ID)
				assert.Equal(t, *tt.expectedComment.Question, *result.Question)
				if tt.expectedComment.Answer != nil {
					assert.Equal(t, *tt.expectedComment.Answer, *result.Answer)
				} else {
					assert.Nil(t, result.Answer)
				}
				assert.Equal(t, tt.expectedComment.Published, result.Published)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestCommentRepository_FindByIDAndTenantWithDetails(t *testing.T) {
	tests := []struct {
		name          string
		commentID     string
		tenantID      string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedResponse *dto.CommentResponse
	}{
		{
			name:      "should return comment response when found",
			commentID: "comment-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				question := "Test question?"
				answer := "Test answer"
				updatedAt := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "updatedAt", "lesson_name", "course_name", "user_name", "user_email",
				}).AddRow(
					"comment-123", question, answer, true, updatedAt, "Lesson 1", "Course 1", "User 1", "user1@test.com",
				)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("comment-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedResponse: &dto.CommentResponse{
				ID:         "comment-123",
				Question:   "Test question?",
				Answer:     "Test answer",
				Published:  true,
				LessonName: "Lesson 1",
				CourseName: "Course 1",
				UserName:   "User 1",
				UserEmail:  "user1@test.com",
			},
		},
		{
			name:      "should return ErrCommentNotFound when comment does not exist",
			commentID: "non-existent",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("non-existent", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError:   memberclasserrors.ErrCommentNotFound,
			expectedResponse: nil,
		},
		{
			name:      "should return error when database error occurs",
			commentID: "comment-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("comment-123", "tenant-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError:   errors.New("database connection error"),
			expectedResponse: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByIDAndTenantWithDetails(context.Background(), tt.commentID, tt.tenantID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedError, memberclasserrors.ErrCommentNotFound) {
					assert.Equal(t, memberclasserrors.ErrCommentNotFound, err)
				} else {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResponse.ID, result.ID)
				assert.Equal(t, tt.expectedResponse.Question, result.Question)
				assert.Equal(t, tt.expectedResponse.Answer, result.Answer)
				assert.Equal(t, tt.expectedResponse.Published, result.Published)
				assert.Equal(t, tt.expectedResponse.LessonName, result.LessonName)
				assert.Equal(t, tt.expectedResponse.CourseName, result.CourseName)
				assert.Equal(t, tt.expectedResponse.UserName, result.UserName)
				assert.Equal(t, tt.expectedResponse.UserEmail, result.UserEmail)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestCommentRepository_Update(t *testing.T) {
	tests := []struct {
		name          string
		commentID     string
		answer        string
		published     bool
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedComment *entities.Comment
	}{
		{
			name:      "should update comment successfully",
			commentID: "comment-123",
			answer:    "Updated answer",
			published: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				question := "Test question?"
				updatedAt := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "updatedAt",
				}).AddRow(
					"comment-123", question, "Updated answer", true, updatedAt,
				)
				sqlMock.ExpectQuery(`UPDATE "Comment"`).
					WithArgs("comment-123", "Updated answer", true, sqlmock.AnyArg()).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedComment: func() *entities.Comment {
				commentID := "comment-123"
				question := "Test question?"
				answer := "Updated answer"
				return &entities.Comment{
					ID:        &commentID,
					Question:  &question,
					Answer:    &answer,
					Published: true,
				}
			}(),
		},
		{
			name:      "should return error when database error occurs",
			commentID: "comment-123",
			answer:    "Updated answer",
			published: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`UPDATE "Comment"`).
					WithArgs("comment-123", "Updated answer", false, sqlmock.AnyArg()).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError:   errors.New("database connection error"),
			expectedComment: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.Update(context.Background(), tt.commentID, tt.answer, tt.published)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expectedComment.ID, *result.ID)
				assert.Equal(t, *tt.expectedComment.Question, *result.Question)
				assert.Equal(t, *tt.expectedComment.Answer, *result.Answer)
				assert.Equal(t, tt.expectedComment.Published, result.Published)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestCommentRepository_FindAllByTenant(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		pagination    *dto.PaginationRequest
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedCount int
		expectedTotal int64
	}{
		{
			name:     "should return comments with pagination",
			tenantID:  "tenant-123",
			pagination: &dto.PaginationRequest{
				Page:     1,
				PageSize: 10,
				SortBy:   "updatedAt",
				SortDir:  "desc",
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				updatedAt1 := time.Now()
				updatedAt2 := time.Now().Add(-1 * time.Hour)

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "updatedAt", "lesson_name", "course_name", "user_name", "user_email",
				}).
					AddRow("comment-1", "Question 1?", "Answer 1", true, updatedAt1, "Lesson 1", "Course 1", "User 1", "user1@test.com").
					AddRow("comment-2", "Question 2?", "Answer 2", false, updatedAt2, "Lesson 2", "Course 2", "User 2", "user2@test.com")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("tenant-123").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 2,
			expectedTotal: 2,
		},
		{
			name:     "should return empty list when no comments found",
			tenantID:  "tenant-123",
			pagination: &dto.PaginationRequest{
				Page:     1,
				PageSize: 10,
				SortBy:   "updatedAt",
				SortDir:  "desc",
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "updatedAt", "lesson_name", "course_name", "user_name", "user_email",
				})

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("tenant-123").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:     "should return error when query fails",
			tenantID:  "tenant-123",
			pagination: &dto.PaginationRequest{
				Page:     1,
				PageSize: 10,
				SortBy:   "updatedAt",
				SortDir:  "desc",
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: errors.New("database connection error"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:     "should return error when count query fails",
			tenantID:  "tenant-123",
			pagination: &dto.PaginationRequest{
				Page:     1,
				PageSize: 10,
				SortBy:   "updatedAt",
				SortDir:  "desc",
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				updatedAt := time.Now()

				rows := sqlmock.NewRows([]string{
					"id", "question", "answer", "published", "updatedAt", "lesson_name", "course_name", "user_name", "user_email",
				}).
					AddRow("comment-1", "Question 1?", "Answer 1", true, updatedAt, "Lesson 1", "Course 1", "User 1", "user1@test.com")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("count query error"))
			},
			expectedError: errors.New("count query error"),
			expectedCount: 0,
			expectedTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			repository := NewCommentRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, total, err := repository.FindAllByTenant(context.Background(), tt.tenantID, tt.pagination)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, result)
				assert.Equal(t, int64(0), total)
			} else {
				assert.NoError(t, err)
				if result != nil {
					assert.Equal(t, tt.expectedCount, len(result))
				} else {
					assert.Equal(t, 0, tt.expectedCount)
				}
				assert.Equal(t, tt.expectedTotal, total)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}
