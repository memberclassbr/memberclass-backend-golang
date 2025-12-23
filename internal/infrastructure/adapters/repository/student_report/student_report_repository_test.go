package student_report

import (
	"context"
	"database/sql"

	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewStudentReportRepository(t *testing.T) {
	t.Run("should create new student report repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewStudentReportRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}

func TestStudentReportRepository_GetStudentsReport(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		startDate     *time.Time
		endDate       *time.Time
		page          int
		limit         int
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedCount int
	}{
		{
			name:      "should return students report successfully",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt).
					AddRow("user-2", "user2@example.com", "98765432100", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow("delivery-1", "Entrega 1").
					AddRow("delivery-2", "Entrega 2")
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"}).
					AddRow("user-1", "delivery-1")
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(sqlmock.AnyArg()).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(sqlmock.AnyArg(), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				lessonsRows := sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"}).
					AddRow("user-1", "lesson-1", "Aula 1", time.Now())
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(sqlmock.AnyArg(), "tenant-123").
					WillReturnRows(lessonsRows)

				lastAccessRows := sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"}).
					AddRow("user-1", time.Now())
				sqlMock.ExpectQuery(`SELECT DISTINCT ON`).
					WithArgs(sqlmock.AnyArg(), "tenant-123").
					WillReturnRows(lastAccessRows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) FROM "UsersOnTenants"`).
					WithArgs("tenant-123").
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 2,
		},
		{
			name:      "should return students report with date filters",
			tenantID:  "tenant-123",
			startDate: timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			endDate:   timePtr(time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)),
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", sqlmock.AnyArg(), sqlmock.AnyArg(), 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				lessonsRows := sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"})
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lessonsRows)

				lastAccessRows := sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"})
				sqlMock.ExpectQuery(`SELECT DISTINCT ON`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lastAccessRows)

				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) FROM "UsersOnTenants"`).
					WithArgs("tenant-123", sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(countRows)
			},
			expectedError: nil,
			expectedCount: 1,
		},
		{
			name:      "should return empty list when no students found",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"})
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedCount: 0,
		},
		{
			name:      "should return error when database error occurs",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error getting students report",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when count query fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				lessonsRows := sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"})
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lessonsRows)

				lastAccessRows := sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"})
				sqlMock.ExpectQuery(`SELECT DISTINCT ON`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lastAccessRows)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) FROM "UsersOnTenants"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("count error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error counting students",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when getDeliveries fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("delivery error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error getting deliveries",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when getUserDeliveries fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnError(errors.New("user delivery error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error getting user deliveries",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when getLessonsWatched fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnError(errors.New("lessons error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error getting lessons watched",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when getLastAccesses fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				lessonsRows := sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"})
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lessonsRows)

				sqlMock.ExpectQuery(`SELECT DISTINCT ON`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnError(errors.New("last access error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error getting last accesses",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when iterating students fails",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", time.Now()).
					CloseError(errors.New("iteration error"))
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error iterating students",
			},
			expectedCount: 0,
		},
		{
			name:      "should return error when count returns sql.ErrNoRows",
			tenantID:  "tenant-123",
			startDate: nil,
			endDate:   nil,
			page:      1,
			limit:     10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				assignedAt := time.Now()
				rows := sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
					AddRow("user-1", "user1@example.com", "12345678900", assignedAt)
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs("tenant-123", 10, 0).
					WillReturnRows(rows)

				deliveryRows := sqlmock.NewRows([]string{"id", "name"})
				sqlMock.ExpectQuery(`SELECT id, name FROM "Delivery"`).
					WithArgs("tenant-123").
					WillReturnRows(deliveryRows)

				userOnDeliveryRows := sqlmock.NewRows([]string{"userId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "userId", "deliveryId" FROM "UserOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"})).
					WillReturnRows(userOnDeliveryRows)

				memberOnDeliveryRows := sqlmock.NewRows([]string{"memberId", "deliveryId"})
				sqlMock.ExpectQuery(`SELECT "memberId", "deliveryId" FROM "MemberOnDelivery"`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(memberOnDeliveryRows)

				lessonsRows := sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"})
				sqlMock.ExpectQuery(`SELECT`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lessonsRows)

				lastAccessRows := sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"})
				sqlMock.ExpectQuery(`SELECT DISTINCT ON`).
					WithArgs(pq.Array([]string{"user-1"}), "tenant-123").
					WillReturnRows(lastAccessRows)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) FROM "UsersOnTenants"`).
					WithArgs("tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError: nil,
			expectedCount: 1,
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

			repository := NewStudentReportRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, totalCount, err := repository.GetStudentsReport(context.Background(), tt.tenantID, tt.startDate, tt.endDate, tt.page, tt.limit)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectedCount == 0 {
					assert.Equal(t, 0, len(result))
				} else {
					assert.Equal(t, tt.expectedCount, len(result))
					if len(result) > 0 {
						assert.NotEmpty(t, result[0].AlunoIDMemberClass)
						assert.NotEmpty(t, result[0].Email)
					}
				}
				assert.GreaterOrEqual(t, totalCount, int64(0))
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
