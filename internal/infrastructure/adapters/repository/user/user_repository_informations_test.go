package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// legacyTableGuardMatcher behaves like the default regexp matcher but also
// fails any query that references the legacy "UserOnDelivery" table. That
// table does not exist in the schema — the deliveries query used to UNION it
// with "MemberOnDelivery", which made GET /api/v1/user/informations return
// 500 `error finding deliveries` (pq: relation "UserOnDelivery" does not exist).
// This guard ensures the regression can never be reintroduced silently.
var legacyTableGuardMatcher = sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
	if strings.Contains(actualSQL, "UserOnDelivery") {
		return fmt.Errorf("query references non-existent legacy table UserOnDelivery: %s", actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
})

func TestUserRepository_FindUserInformations(t *testing.T) {
	tenantID := "tenant-1"

	t.Run("should return users with deliveries and not touch legacy UserOnDelivery table", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(legacyTableGuardMatcher))
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserRepository(db, mockLogger)

		// 1) count
		sqlMock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WithArgs(tenantID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

		// 2) users_base CTE + aggregation
		assignedAt := time.Now()
		sqlMock.ExpectQuery(`WITH\s+users_base AS`).
			WithArgs(tenantID, 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}).
				AddRow("user-1", "u1@example.com", "User One", true, assignedAt))

		// 3) deliveries — must query MemberOnDelivery, never UserOnDelivery
		sqlMock.ExpectQuery(`FROM "MemberOnDelivery" mod`).
			WithArgs(sqlmock.AnyArg(), tenantID).
			WillReturnRows(sqlmock.NewRows([]string{"memberId", "deliveryId", "assignedAt", "delivery_name"}).
				AddRow("user-1", "del-1", assignedAt, "Delivery One"))

		result, total, err := repository.FindUserInformations(context.Background(), tenantID, "", 1, 10)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, result, 1)
		assert.Equal(t, "user-1", result[0].UserID)
		assert.Equal(t, "u1@example.com", result[0].Email)
		assert.True(t, result[0].IsPaid)
		assert.Len(t, result[0].Deliveries, 1)
		assert.Equal(t, "del-1", result[0].Deliveries[0].ID)
		assert.Equal(t, "Delivery One", result[0].Deliveries[0].Name)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("should filter by email and pass it as a query argument", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(legacyTableGuardMatcher))
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserRepository(db, mockLogger)

		email := "u1@example.com"

		sqlMock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WithArgs(tenantID, email).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

		sqlMock.ExpectQuery(`WITH\s+users_base AS`).
			WithArgs(tenantID, 10, 0, email).
			WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}).
				AddRow("user-1", email, "User One", false, nil))

		sqlMock.ExpectQuery(`FROM "MemberOnDelivery" mod`).
			WithArgs(sqlmock.AnyArg(), tenantID).
			WillReturnRows(sqlmock.NewRows([]string{"memberId", "deliveryId", "assignedAt", "delivery_name"}))

		result, total, err := repository.FindUserInformations(context.Background(), tenantID, email, 1, 10)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, result, 1)
		assert.Equal(t, email, result[0].Email)
		assert.Len(t, result[0].Deliveries, 0)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("should return empty result and skip deliveries query when no users match", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(legacyTableGuardMatcher))
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewUserRepository(db, mockLogger)

		sqlMock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WithArgs(tenantID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

		sqlMock.ExpectQuery(`WITH\s+users_base AS`).
			WithArgs(tenantID, 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}))

		result, total, err := repository.FindUserInformations(context.Background(), tenantID, "", 1, 10)

		assert.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, result, 0)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("should return error finding deliveries when deliveries query fails", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(legacyTableGuardMatcher))
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		mockLogger.EXPECT().Error(mock.Anything).Return()
		repository := NewUserRepository(db, mockLogger)

		sqlMock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WithArgs(tenantID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

		sqlMock.ExpectQuery(`WITH\s+users_base AS`).
			WithArgs(tenantID, 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}).
				AddRow("user-1", "u1@example.com", "User One", true, nil))

		sqlMock.ExpectQuery(`FROM "MemberOnDelivery" mod`).
			WithArgs(sqlmock.AnyArg(), tenantID).
			WillReturnError(errors.New("boom"))

		result, total, err := repository.FindUserInformations(context.Background(), tenantID, "", 1, 10)

		assert.Nil(t, result)
		assert.Equal(t, int64(0), total)
		assert.Equal(t, &memberclasserrors.MemberClassError{Code: 500, Message: "error finding deliveries"}, err)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})
}
