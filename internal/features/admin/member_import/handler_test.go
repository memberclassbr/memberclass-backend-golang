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
	"github.com/memberclass-backend-golang/internal/application/middlewares/auth"
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
	u := &auth.AuthUser{
		UserID: userID,
		Email:  "admin@example.com",
		Role:   "owner",
		Exp:    time.Now().Add(time.Hour).Unix(),
	}
	return r.WithContext(auth.ContextWithAuthUser(r.Context(), u))
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

func TestImportMembers_RoleMember_Forbidden(t *testing.T) {
	f, mock, done := newFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u-1", "t-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))

	w := doImport(f, importRequest{
		TenantID: "t-1",
		Users:    []importUserInput{{Name: "x", Email: "x@y.com"}},
	}, "u-1")

	assert.Equal(t, http.StatusForbidden, w.Code)
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
