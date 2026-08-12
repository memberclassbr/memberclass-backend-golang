package tenantrole

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newChecker(t *testing.T) (*Checker, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return New(db), mock, func() { _ = db.Close() }
}

// ctxAs builds the context the Bearer middleware would have produced. The role
// on the token is set to something generous on purpose: no test here may pass
// because of it.
func ctxAs(userID string) context.Context {
	return middleware.ContextWithAuthUser(context.Background(), &middleware.AuthUser{
		UserID: userID,
		Email:  userID + "@example.com",
		Role:   Owner,
	})
}

func expectRole(mock sqlmock.Sqlmock, userID, tenantID, role string) {
	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs(userID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(role))
}

// ---------- Identity ----------

func TestAuthorize_WithoutIdentity(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	_, err := c.Authorize(context.Background(), "t1", AnyRole...)

	assert.ErrorIs(t, err, ErrNoIdentity)
	// The database was never touched.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthorize_WithEmptyTenant(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	_, err := c.Authorize(ctxAs("u1"), "", AnyRole...)

	assert.ErrorIs(t, err, ErrNotMember)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- The role comes from the database, not the token ----------

// This is the whole point of the package: the token above claims owner, and
// the caller is still refused an owner-only action.
func TestAuthorize_IgnoresTheRoleClaimOnTheToken(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	expectRole(mock, "u1", "t1", Member)

	_, err := c.Authorize(ctxAs("u1"), "t1", OwnerOrAdmin...)

	assert.ErrorIs(t, err, ErrForbiddenRole)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The lookup is scoped by tenant, so being an owner elsewhere buys nothing
// here.
func TestAuthorize_ScopesTheLookupByTenant(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u1", "other-tenant").
		WillReturnError(sql.ErrNoRows)

	_, err := c.Authorize(ctxAs("u1"), "other-tenant", AnyRole...)

	assert.ErrorIs(t, err, ErrNotMember)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- Whitelists ----------

func TestAuthorize_OwnerOrAdmin(t *testing.T) {
	cases := map[string]error{
		Owner:   nil,
		Admin:   nil,
		Manager: ErrForbiddenRole,
		Tutor:   ErrForbiddenRole,
		Member:  ErrForbiddenRole,
	}

	for role, want := range cases {
		t.Run(role, func(t *testing.T) {
			c, mock, done := newChecker(t)
			defer done()

			expectRole(mock, "u1", "t1", role)

			grant, err := c.Authorize(ctxAs("u1"), "t1", OwnerOrAdmin...)

			if want != nil {
				assert.ErrorIs(t, err, want)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, role, grant.Role)
			assert.Equal(t, "u1", grant.UserID)
			assert.Equal(t, "t1", grant.TenantID)
		})
	}
}

func TestAuthorize_AnyRoleAcceptsEveryRole(t *testing.T) {
	for _, role := range []string{Owner, Admin, Manager, Tutor, Member} {
		t.Run(role, func(t *testing.T) {
			c, mock, done := newChecker(t)
			defer done()

			expectRole(mock, "u1", "t1", role)

			grant, err := c.Authorize(ctxAs("u1"), "t1", AnyRole...)

			require.NoError(t, err)
			assert.Equal(t, role, grant.Role)
		})
	}
}

// A membership row whose role is blank is not a wildcard — the column is a
// free String in Prisma, so an empty value is reachable.
func TestAuthorize_EmptyRoleIsNotAWildcard(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	expectRole(mock, "u1", "t1", "")

	_, err := c.Authorize(ctxAs("u1"), "t1", AnyRole...)

	assert.ErrorIs(t, err, ErrForbiddenRole)
}

// ---------- Failure mapping ----------

// A lookup that failed for an infrastructure reason must not read as a denial:
// answering 403 would tell an owner they had been demoted.
func TestAuthorize_DatabaseFailurePropagates(t *testing.T) {
	c, mock, done := newChecker(t)
	defer done()

	boom := errors.New("connection reset")
	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).WillReturnError(boom)

	_, err := c.Authorize(ctxAs("u1"), "t1", AnyRole...)

	assert.ErrorIs(t, err, boom)
	assert.Equal(t, http.StatusInternalServerError, Status(err))
}

func TestStatus(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, Status(ErrNoIdentity))
	assert.Equal(t, http.StatusForbidden, Status(ErrNotMember))
	assert.Equal(t, http.StatusForbidden, Status(ErrForbiddenRole))
	assert.Equal(t, http.StatusInternalServerError, Status(errors.New("anything else")))
}
