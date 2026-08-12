package member_import

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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
	return New(db, fakeLogger{}, nil), mock, func() { _ = db.Close() }
}

func withUserSession(r *http.Request, userID string) *http.Request {
	u := &middleware.AuthUser{
		UserID: userID,
		Email:  "admin@example.com",
		Role:   "owner",
		Exp:    time.Now().Add(time.Hour).Unix(),
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

func TestImportMembers_MissingTenantID(t *testing.T) {
	f, _, done := newFeature(t)
	defer done()

	w := doImport(f, importRequest{Users: []importUserInput{{Name: "x", Email: "x@y.com"}}}, "u-1")

	assert.Equal(t, http.StatusBadRequest, w.Code)
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
		UserID: "u-1",
		Role:   "owner", // the claim says owner; the database says member
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

func TestPickProtocol(t *testing.T) {
	assert.Equal(t, "http", pickProtocol("localhost:3000"))
	assert.Equal(t, "https", pickProtocol("acme.memberclass.com.br"))
}

func TestBuildMagicLink(t *testing.T) {
	t.Run("uses shortCode when present", func(t *testing.T) {
		got := buildMagicLink("https", "acme.com", "ABC123", "long-raw-token", "user@example.com")
		assert.Contains(t, got, "https://acme.com/login?")
		assert.Contains(t, got, "code=ABC123")
		assert.NotContains(t, got, "token=")
	})
	t.Run("falls back to token+email without shortCode", func(t *testing.T) {
		got := buildMagicLink("https", "acme.com", "", "tok", "user@example.com")
		assert.Contains(t, got, "token=tok")
		assert.Contains(t, got, "email=user%40example.com")
	})
}

// nullStr is a tiny helper to build sql.NullString literals inline.
func nullStr(s string) sql.NullString { return sql.NullString{Valid: true, String: s} }
