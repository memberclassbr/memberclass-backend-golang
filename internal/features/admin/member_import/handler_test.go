package member_import

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// ---------- Helpers ----------

func newFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	// resend service isn't hit by these tests (they stop before the worker).
	cfg := &config.Config{Public: config.Public{DomainURL: "memberclass.com.br"}}
	return New(db, fakeLogger{}, nil, cfg), mock, func() { _ = db.Close() }
}

// withUserSession builds the context the Bearer middleware would have produced.
// The role on the token is generous on purpose: no test may pass because of it.
func withUserSession(r *http.Request, userID string) *http.Request {
	u := &middleware.AuthUser{
		UserID:   userID,
		Email:    "admin@example.com",
		TenantID: "t-1",
		Role:     "owner",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	return r.WithContext(middleware.ContextWithAuthUser(r.Context(), u))
}

func doImport(f *Feature, body importRequest, userID string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := withUserSession(
		httptest.NewRequest(http.MethodPost, "/imports/members", bytes.NewReader(raw)),
		userID,
	)
	w := httptest.NewRecorder()
	f.ImportMembers(w, req)
	return w
}

// ---------- Validation ----------

func TestImportMembers_InvalidJSON(t *testing.T) {
	f, _, done := newFeature(t)
	defer done()

	w := httptest.NewRecorder()
	req := withUserSession(
		httptest.NewRequest(http.MethodPost, "/imports/members", bytes.NewReader([]byte("not-json"))),
		"u-1",
	)
	f.ImportMembers(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The body no longer has to carry a tenantId — the token names one. Omitting it
// is the ordinary request now, not a validation failure.
func TestImportMembers_TenantIDIsOptionalInTheBody(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u-1", "t-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("owner"))
	// Passing the gate is proved by failing the very next query rather than by
	// reaching 202, which would spawn the worker against a closing mock.
	mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(sql.ErrConnDone)

	w := doImport(f, importRequest{Users: []importUserInput{{Name: "x", Email: "x@y.com"}}}, "u-1")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A body naming a tenant the token was not minted for is refused, not silently
// redirected into the token's tenant. The claim is the source either way; this
// is what tells a caller its field was wrong.
func TestImportMembers_RejectsATenantTheTokenDoesNotName(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u-1", "t-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("owner"))

	w := doImport(f, importRequest{
		TenantID: "someone-elses-tenant",
		Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
	}, "u-1")

	assert.Equal(t, http.StatusForbidden, w.Code)
	// Nothing was loaded or written for the tenant the body named.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestImportMembers_EmptyUsers(t *testing.T) {
	f, _, done := newFeature(t)
	defer done()

	w := doImport(f, importRequest{TenantID: "t-1"}, "u-1")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMembers_MissingSession(t *testing.T) {
	f, _, done := newFeature(t)
	defer done()

	raw, _ := json.Marshal(importRequest{
		TenantID: "t-1",
		Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/imports/members", bytes.NewReader(raw))
	f.ImportMembers(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- Authorization ----------

func TestImportMembers_UserNotInTenant(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	// Role lookup returns no rows → 403.
	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u-1", "t-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"})) // empty

	w := doImport(f, importRequest{
		TenantID: "t-1",
		Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
	}, "u-1")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// Bulk import is owner/admin only. The rule used to be "anything but member",
// which let tutor and manager through; they no longer pass.
func TestImportMembers_OnlyOwnerAndAdminMayImport(t *testing.T) {
	// The allowed roles are asserted as "got past the role check" rather than
	// as 202: letting the handler reach 202 would spawn the import worker
	// against a mock that is about to be closed. Failing the very next query —
	// the tenant load — proves the gate opened without starting any work.
	const passedTheGate = http.StatusInternalServerError

	cases := map[string]int{
		"owner":   passedTheGate,
		"admin":   passedTheGate,
		"manager": http.StatusForbidden,
		"tutor":   http.StatusForbidden,
		"member":  http.StatusForbidden,
		// A row that exists with no role at all is not a wildcard.
		"": http.StatusForbidden,
	}

	for role, want := range cases {
		t.Run("role="+role, func(t *testing.T) {
			f, mock, done := newFeature(t)
			defer done()

			mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
				WithArgs("u-1", "t-1").
				WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(role))

			if want == passedTheGate {
				mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(sql.ErrConnDone)
			}

			w := doImport(f, importRequest{
				TenantID: "t-1",
				Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
			}, "u-1")

			assert.Equal(t, want, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The Bearer's own `role` claim is never consulted: a token minted while the
// account was an owner must not survive the demotion that follows it.
func TestImportMembers_IgnoresTheRoleClaimOnTheToken(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u-1", "t-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))

	raw, err := json.Marshal(importRequest{
		TenantID: "t-1",
		Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/imports/members", bytes.NewReader(raw))
	req = req.WithContext(middleware.ContextWithAuthUser(req.Context(), &middleware.AuthUser{
		UserID:   "u-1",
		TenantID: "t-1",
		Role:     "owner", // the claim says owner; the database says member
	}))

	w := httptest.NewRecorder()
	f.ImportMembers(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- Helpers (unit) ----------

func TestTenantDomain(t *testing.T) {
	t.Run("customDomain wins", func(t *testing.T) {
		got := tenantDomain(&tenantRow{
			Subdomain:    nullStr("sub"),
			CustomDomain: nullStr("custom.com"),
		}, "root.com")
		assert.Equal(t, "custom.com", got)
	})
	t.Run("falls back to subdomain", func(t *testing.T) {
		got := tenantDomain(&tenantRow{Subdomain: nullStr("acme")}, "memberclass.com.br")
		assert.Equal(t, "acme.memberclass.com.br", got)
	})
	t.Run("falls back to root when no subdomain", func(t *testing.T) {
		got := tenantDomain(&tenantRow{}, "memberclass.com.br")
		assert.Equal(t, "memberclass.com.br", got)
	})
}

// createMagicToken stores a sha256 digest in "MagicToken"."token" — the bcrypt
// hash this slice uses for "User"."magicToken" would never match what the
// frontend computes for the same token.
func TestCreateMagicTokenStoresADigest(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	var stored string
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WithArgs(
			sqlmock.AnyArg(), // id
			capture(&stored),
			sqlmock.AnyArg(), // shortCode
			"u1", "t1",
			sqlmock.AnyArg(), // expires
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	shortCode, err := f.createMagicToken(
		context.Background(), "u1", "t1", "raw-token", "user@example.com", time.Now().Add(time.Hour),
	)
	require.NoError(t, err)

	sum := sha256.Sum256([]byte("raw-token"))
	assert.Equal(t, hex.EncodeToString(sum[:]), stored)
	assert.Len(t, shortCode, 12)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A collision on the unique "shortCode" index is regenerated. The row failing
// outright would mark the import row as an error and send no email at all.
func TestCreateMagicTokenRetriesOnCollision(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "MagicToken_shortCode_key"})
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	shortCode, err := f.createMagicToken(
		context.Background(), "u1", "t1", "raw-token", "user@example.com", time.Now().Add(time.Hour),
	)
	require.NoError(t, err)
	assert.Len(t, shortCode, 12)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// capture is a sqlmock argument matcher that accepts any string and records it.
type capturingArg struct{ into *string }

func (c capturingArg) Match(v driver.Value) bool {
	s, ok := v.(string)
	if ok {
		*c.into = s
	}
	return ok
}

func capture(into *string) sqlmock.Argument { return capturingArg{into: into} }

// nullStr is a tiny helper to build sql.NullString literals inline.
func nullStr(s string) sql.NullString { return sql.NullString{Valid: true, String: s} }
