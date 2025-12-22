package user

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUserRepository_FindPurchasesByUserAndTenant(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		tenantID        string
		purchaseTypes   []string
		page            int
		limit           int
		mockSetup       func(sqlmock.Sqlmock)
		expectedError   error
		expectedCount   int
		expectedTotal   int64
	}{
		{
			name:          "should return purchases when found",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
					AddRow("event-123", "purchase", "2024-01-15T10:30:00.000Z", "2024-01-15T10:30:00.000Z", 2).
					AddRow("event-456", "refund", "2024-01-14T15:20:00.000Z", "2024-01-14T15:20:00.000Z", 2)

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 0).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedCount: 2,
			expectedTotal: 2,
		},
		{
			name:          "should return empty list when no purchases found",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"})

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 0).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:          "should filter by purchase type",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{"purchase"},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
					AddRow("event-123", "purchase", "2024-01-15T10:30:00.000Z", "2024-01-15T10:30:00.000Z", 1)

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase"}), 10, 0).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedCount: 1,
			expectedTotal: 1,
		},
		{
			name:          "should handle pagination correctly",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          2,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
					AddRow("event-11", "purchase", "2024-01-15T10:30:00.000Z", "2024-01-15T10:30:00.000Z", 25).
					AddRow("event-12", "refund", "2024-01-14T15:20:00.000Z", "2024-01-14T15:20:00.000Z", 25)

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 10).
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedCount: 2,
			expectedTotal: 25,
		},
		{
			name:          "should return error when query fails",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 0).
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: errors.New("error finding purchases"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:          "should return error when scanning fails",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
					AddRow(nil, nil, nil, nil, nil)

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 0).
					WillReturnRows(rows)
			},
			expectedError: errors.New("error scanning purchase"),
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:          "should return error when rows iteration fails",
			userID:        "user-123",
			tenantID:      "tenant-123",
			purchaseTypes: []string{},
			page:          1,
			limit:         10,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
					AddRow("event-123", "purchase", "2024-01-15T10:30:00.000Z", "2024-01-15T10:30:00.000Z", 1).
					RowError(0, errors.New("row error"))

				sqlMock.ExpectQuery(`WITH filtered AS`).
					WithArgs("user-123", "tenant-123", pq.Array([]string{"purchase", "refund"}), 10, 0).
					WillReturnRows(rows)
			},
			expectedError: errors.New("error iterating purchases"),
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
			mockLogger.On("Error", mock.AnythingOfType("string")).Return().Maybe()

			repository := NewUserRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, total, err := repository.FindPurchasesByUserAndTenant(context.Background(), tt.userID, tt.tenantID, tt.purchaseTypes, tt.page, tt.limit)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if memberClassErr, ok := err.(*memberclasserrors.MemberClassError); ok {
					assert.Contains(t, memberClassErr.Message, tt.expectedError.Error())
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedCount, len(result))
				if tt.expectedCount > 0 {
					assert.Equal(t, tt.expectedTotal, total)
				} else {
					assert.Equal(t, int64(0), total)
				}
			}

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}
