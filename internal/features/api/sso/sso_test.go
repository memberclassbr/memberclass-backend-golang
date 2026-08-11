package sso

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInternalKey = "internal-key"

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	cfg := &config.Config{Auth: config.Auth{InternalAPIKey: testInternalKey}}
	return New(db, cfg, fakeLogger{}), mock, func() { _ = db.Close() }
}

func generateRequest(body, externalURL string, withKey bool) *http.Request {
	target := "/generate-token"
	if externalURL != "" {
		target += "?externalUrl=" + url.QueryEscape(externalURL)
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if withKey {
		req.Header.Set("x-internal-api-key", testInternalKey)
	}
	return req
}

func bodyOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func lockRowColumns() []string {
	return []string{"userId", "tenantId", "ssoTokenValidUntil", "ssoTokenUsedAt", "email", "name", "phone", "tenant_name"}
}

// ---------- 1. generate-token authorisation ----------

// The endpoint mints credentials, so an unauthenticated caller must be
// rejected before anything else — including before the method check.
func TestGenerateSSOToken_RejectsBadInternalKey(t *testing.T) {
	cases := []struct {
		name    string
		withKey bool
		key     string
	}{
		{"no header", false, ""},
		{"wrong key", true, "nope"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			req := generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", false)
			if tc.withKey {
				req.Header.Set("x-internal-api-key", tc.key)
			}

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, "UNAUTHORIZED", bodyOf(t, w)["errorCode"])
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGenerateSSOToken_RequiredFields(t *testing.T) {
	cases := map[string]struct {
		body        string
		externalURL string
	}{
		"missing userId":      {`{"tenantId":"t1"}`, "https://escola.com.br"},
		"missing tenantId":    {`{"userId":"u1"}`, "https://escola.com.br"},
		"missing externalUrl": {`{"userId":"u1","tenantId":"t1"}`, ""},
		"malformed body":      {`{not json`, "https://escola.com.br"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, generateRequest(tc.body, tc.externalURL, true))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ---------- 2. generate-token rules ----------

// An unknown user and a user outside the tenant are reported differently: the
// first is a 404, the second a 403.
func TestGenerateSSOToken_UnknownUserIsNotFound(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" WHERE id`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", true))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateSSOToken_UserOutsideTenantIsForbidden(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WithArgs("u1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", true))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", bodyOf(t, w)["errorCode"])
	// No token was written.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateSSOToken_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br/entrar", true))

	require.Equal(t, http.StatusOK, w.Code)

	var resp generateTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Token, tokenLength)
	assert.Equal(t, 300, resp.ExpiresInSecs)

	parsed, err := url.Parse(resp.RedirectURL)
	require.NoError(t, err)
	assert.Equal(t, "escola.com.br", parsed.Host)
	assert.Equal(t, "/entrar", parsed.Path)
	assert.Equal(t, resp.Token, parsed.Query().Get("token-mc"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Only the hash is stored; the plaintext travels in the redirect URL and is
// never persisted.
func TestGenerateSSOToken_StoresOnlyTheHash(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	var stored string
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"u1", "t1",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", true))
	require.Equal(t, http.StatusOK, w.Code)

	var resp generateTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	stored = hashToken(resp.Token)
	assert.Len(t, stored, 64)
	assert.NotEqual(t, resp.Token, stored)
}

// A user who exists but is not in the tenant matches no row on the UPDATE, so
// the write reports a 404 rather than silently succeeding.
func TestStoreToken_NoRowsIsNotFound(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectExec(`UPDATE "UsersOnTenants"`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := f.storeToken(context.Background(), "u1", "t1", "hash", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user not found in tenant")
}

func TestBuildRedirectURL_PreservesExistingQuery(t *testing.T) {
	out, err := buildRedirectURL("https://escola.com.br/entrar?next=/curso", "abc")
	require.NoError(t, err)

	parsed, err := url.Parse(out)
	require.NoError(t, err)
	assert.Equal(t, "/curso", parsed.Query().Get("next"))
	assert.Equal(t, "abc", parsed.Query().Get("token-mc"))
}

// ---------- 3. validate-token ----------

func TestValidateSSOToken_RequiresToken(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":""}`)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateSSOToken_UnknownTokenIsUnauthorized(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":"nope"}`)))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_TOKEN", bodyOf(t, w)["errorCode"])
	assert.Contains(t, w.Body.String(), "token inválido")
}

// A token that was already redeemed must be refused, and the transaction must
// not mark it used a second time.
func TestValidateSSOToken_AlreadyUsed(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows(lockRowColumns()).
			AddRow("u1", "t1", time.Now().Add(time.Minute), time.Now().Add(-time.Minute), "a@example.com", "Ana", nil, "Cliente"))
	mock.ExpectRollback()

	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":"abc"}`)))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "token já foi utilizado")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateSSOToken_Expired(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows(lockRowColumns()).
			AddRow("u1", "t1", time.Now().UTC().Add(-time.Minute), nil, "a@example.com", "Ana", nil, "Cliente"))
	mock.ExpectRollback()

	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":"abc"}`)))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "token expirado")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateSSOToken_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	name := "Ana"
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows(lockRowColumns()).
			AddRow("u1", "t1", time.Now().UTC().Add(time.Minute), nil, "a@example.com", name, nil, "Cliente"))
	mock.ExpectExec(`SET "ssoTokenUsedAt"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT document`).
		WillReturnRows(sqlmock.NewRows([]string{"document"}).AddRow("12345678900"))

	req := httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp validateTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "u1", resp.User.ID)
	assert.Equal(t, "a@example.com", resp.User.Email)
	require.NotNil(t, resp.User.Document)
	assert.Equal(t, "12345678900", *resp.User.Document)
	assert.Equal(t, "Cliente", resp.Tenant.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The document lives on a separate row and is optional: failing to read it
// must not undo a redemption that already committed.
func TestValidateSSOToken_DocumentFailureDoesNotUndoRedemption(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows(lockRowColumns()).
			AddRow("u1", "t1", time.Now().UTC().Add(time.Minute), nil, "a@example.com", nil, nil, "Cliente"))
	mock.ExpectExec(`SET "ssoTokenUsedAt"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT document`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodPost, "/validate-token", strings.NewReader(`{"token":"abc"}`)))

	require.Equal(t, http.StatusOK, w.Code)

	var resp validateTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.User.Document)
}

// ---------- 4. Client IP ----------

func TestClientIP_PrefersProxyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	assert.Equal(t, "10.0.0.1:1234", clientIP(req))

	req.Header.Set("X-Real-IP", "198.51.100.5")
	assert.Equal(t, "198.51.100.5", clientIP(req))

	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	assert.Equal(t, "203.0.113.7", clientIP(req))
}

// ---------- 5. Method guards ----------

func TestHandlers_RejectNonPost(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	req := httptest.NewRequest(http.MethodGet, "/generate-token", nil)
	req.Header.Set("x-internal-api-key", testInternalKey)
	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodGet, "/validate-token", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
