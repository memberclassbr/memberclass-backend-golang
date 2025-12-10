package user_activity

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewUserActivityRepository(t *testing.T) {
	t.Run("should create new user activity repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserActivityRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}

func TestUserActivityRepository_FindActivitiesByEmail(t *testing.T) {
	tests := []struct {
		name            string
		email           string
		page            int
		limit           int
		mockSetup       func(sqlmock.Sqlmock)
		expectedError   error
		expectedCount   int
		expectedTotal   int64
	}{
		{
			name:  "should return activities when found",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow("2025-12-10T10:00:00Z").
					AddRow("2025-12-10T09:00:00Z").
					AddRow("2025-12-10T08:00:00Z")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(3)
				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("test@example.com").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 3,
			expectedTotal: 3,
		},
		{
			name:  "should return empty list when no activities found",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"})

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("test@example.com").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:  "should handle pagination correctly",
			email: "test@example.com",
			page:  2,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow("2025-12-10T07:00:00Z").
					AddRow("2025-12-10T06:00:00Z")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 10).
					WillReturnRows(rows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(25)
				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("test@example.com").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 2,
			expectedTotal: 25,
		},
		{
			name:  "should return error when query fails",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: errors.New("error finding activities"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:  "should return error when scanning fails",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow(nil)

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)
			},
			expectedError: errors.New("error scanning activity"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:  "should return error when rows iteration fails",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow("2025-12-10T10:00:00Z").
					RowError(0, errors.New("row error"))

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)
			},
			expectedError: errors.New("error iterating activities"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:  "should return error when count query fails",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow("2025-12-10T10:00:00Z")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("test@example.com").
					WillReturnError(errors.New("count query error"))
			},
			expectedError: errors.New("error counting activities"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:  "should return activities with zero total when count returns no rows",
			email: "test@example.com",
			page:  1,
			limit: 10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"data"}).
					AddRow("2025-12-10T10:00:00Z")

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("test@example.com", 10, 0).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT COUNT`).
					WithArgs("test@example.com").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError: nil,
			expectedCount: 1,
			expectedTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				mockLogger.On("Error", mock.AnythingOfType("string")).Return()
			}

			repository := NewUserActivityRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, total, err := repository.FindActivitiesByEmail(context.Background(), tt.email, tt.page, tt.limit)

			if tt.expectedError != nil {
				assert.Error(t, err)
				var memberClassErr *memberclasserrors.MemberClassError
				if errors.As(err, &memberClassErr) {
					assert.Contains(t, memberClassErr.Message, tt.expectedError.Error())
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
				assert.Nil(t, result)
				assert.Equal(t, int64(0), total)
			} else {
				assert.NoError(t, err)
				if result != nil {
					assert.Equal(t, tt.expectedCount, len(result))
					if tt.expectedCount > 0 {
						assert.NotEmpty(t, result[0].Data)
					}
				} else {
					assert.Equal(t, 0, tt.expectedCount)
				}
				assert.Equal(t, tt.expectedTotal, total)
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
			mockLogger.AssertExpectations(t)
		})
	}
}

func TestUserActivityRepository_FindActivitiesByEmail_EmptySlice(t *testing.T) {
	db, sqlMock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mockLogger := mocks.NewMockLogger(t)
	repository := NewUserActivityRepository(db, mockLogger)

	rows := sqlmock.NewRows([]string{"data"})

	sqlMock.ExpectQuery(`SELECT`).
		WithArgs("test@example.com", 10, 0).
		WillReturnRows(rows)

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	sqlMock.ExpectQuery(`SELECT COUNT`).
		WithArgs("test@example.com").
		WillReturnRows(countRows)

	result, total, err := repository.FindActivitiesByEmail(context.Background(), "test@example.com", 1, 10)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result))
	assert.Equal(t, int64(0), total)
	assert.NotNil(t, result)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
