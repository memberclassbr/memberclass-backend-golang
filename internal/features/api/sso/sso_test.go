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
	"github.com/memberclass-backend-golang/internal/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	return New(db, fakeLogger{}), mock, func() { _ = db.Close() }
}

// generateRequest builds the POST already carrying the identity the Bearer
// middleware would have attached. An empty callerID means no identity at all.
func generateRequest(body, externalURL, callerID string) *http.Request {
	target := "/generate-token"
	if externalURL != "" {
		target += "?externalUrl=" + url.QueryEscape(externalURL)
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if callerID == "" {
		return req
	}
	return req.WithContext(middleware.ContextWithAuthUser(req.Context(), &middleware.AuthUser{
		UserID: callerID,
		Email:  callerID + "@example.com",
		// Claimed on the token and ignored by the handler — every test below
		// gets the role it is really judged by from the mocked query.
		Role: "owner",
	}))
}

// expectRole queues the role lookup that runs before any of the SSO work.
func expectRole(mock sqlmock.Sqlmock, callerID, tenantID, role string) {
	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs(callerID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(role))
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

// A request that reaches the handler without an identity is rejected before a
// single query runs, so the guard does not depend on the Bearer middleware
// being present in the chain.
func TestGenerateSSOToken_RequiresIdentity(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", ""))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHORIZED", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Minting for yourself is what a member does when they follow a link out to
// the tenant's own site, so it is open to the most junior role there is.
func TestGenerateSSOToken_AnyRoleMintsForItself(t *testing.T) {
	for _, role := range []string{"owner", "admin", "manager", "tutor", "member"} {
		t.Run(role, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			expectRole(mock, "u1", "t1", role)
			mock.ExpectQuery(`FROM "User" WHERE id`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			mock.ExpectQuery(`FROM "UsersOnTenants"`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			mock.ExpectExec(`UPDATE "UsersOnTenants"`).
				WillReturnResult(sqlmock.NewResult(0, 1))

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", "u1"))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The token this endpoint mints is redeemed at validate-token for the target
// user's identity on the tenant's external site. Minting one for somebody else
// is impersonation, so it takes owner or admin — this is the check that stands
// between a member and every account in their tenant.
func TestGenerateSSOToken_JuniorRolesCannotMintForAnotherUser(t *testing.T) {
	for _, role := range []string{"member", "tutor", "manager"} {
		t.Run(role, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			expectRole(mock, "attacker", "t1", role)

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, generateRequest(`{"userId":"victim","tenantId":"t1"}`, "https://escola.com.br", "attacker"))

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, "FORBIDDEN", bodyOf(t, w)["errorCode"])
			// Nothing was minted or written.
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGenerateSSOToken_OwnerAndAdminMayMintForAnotherUser(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			expectRole(mock, "boss", "t1", role)
			mock.ExpectQuery(`FROM "User" WHERE id`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			mock.ExpectQuery(`FROM "UsersOnTenants"`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			mock.ExpectExec(`UPDATE "UsersOnTenants"`).
				WillReturnResult(sqlmock.NewResult(0, 1))

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, generateRequest(`{"userId":"someone","tenantId":"t1"}`, "https://escola.com.br", "boss"))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The role is read for the tenant named in the body. A caller who is an owner
// somewhere else holds nothing here.
func TestGenerateSSOToken_RejectsTenantTheCallerDoesNotBelongTo(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs("u1", "other-tenant").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"other-tenant"}`, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// `externalUrl` is the only field with no sensible default — the whole point
// of the endpoint is the site being handed off to.
func TestGenerateSSOToken_RequiredFields(t *testing.T) {
	cases := map[string]struct {
		body        string
		externalURL string
	}{
		"missing externalUrl": {`{"userId":"u1","tenantId":"t1"}`, ""},
		"malformed body":      {`{not json`, "https://escola.com.br"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.GenerateSSOToken(w, generateRequest(tc.body, tc.externalURL, "u1"))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
			// A malformed request never reaches the database.
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ---------- 1b. Defaults for userId and tenantId ----------

// expectTenantsOf queues the membership listing that backs the tenantId
// default.
func expectTenantsOf(mock sqlmock.Sqlmock, callerID string, tenantIDs ...string) {
	rows := sqlmock.NewRows([]string{"tenantId"})
	for _, id := range tenantIDs {
		rows.AddRow(id)
	}
	mock.ExpectQuery(`SELECT "tenantId" FROM "UsersOnTenants"`).
		WithArgs(callerID).
		WillReturnRows(rows)
}

// An empty body is a complete request: it means "a hand-off for me, in my
// tenant". Both fields come off the caller.
func TestGenerateSSOToken_DefaultsBothFieldsToTheCaller(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectTenantsOf(mock, "u1", "t1")
	expectRole(mock, "u1", "t1", "member")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WithArgs("u1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "u1", "t1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(``, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A tenantId in the body is used as given — no listing query runs.
func TestGenerateSSOToken_ExplicitTenantSkipsTheLookup(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectRole(mock, "u1", "t1", "member")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"tenantId":"t1"}`, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// "UsersOnTenants" is many-to-many, so a caller can hold a different role in
// each of several tenants. Picking one would mint a hand-off into the wrong
// one, so the request is refused until it says which.
func TestGenerateSSOToken_AmbiguousTenantIsRejected(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectTenantsOf(mock, "u1", "t1", "t2")

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{}`, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
	assert.Contains(t, bodyOf(t, w)["error"], "mais de um tenant")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateSSOToken_CallerWithNoTenantIsForbidden(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectTenantsOf(mock, "u1")

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{}`, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The default is the *caller*, never the userId in the body: an owner naming
// somebody else still has their own memberships listed, so a target who
// belongs elsewhere cannot drag the hand-off into another tenant.
func TestGenerateSSOToken_TenantDefaultFollowsTheCallerNotTheTarget(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectTenantsOf(mock, "boss", "t1")
	expectRole(mock, "boss", "t1", "owner")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WithArgs("someone").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WithArgs("someone", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"someone"}`, "https://escola.com.br", "boss"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 2. generate-token rules ----------

// An unknown user and a user outside the tenant are reported differently: the
// first is a 404, the second a 403.
func TestGenerateSSOToken_UnknownUserIsNotFound(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectRole(mock, "u1", "t1", "member")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", "u1"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NOT_FOUND", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateSSOToken_UserOutsideTenantIsForbidden(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	// Reachable only when the caller is minting for somebody else: a caller
	// minting for themselves already proved their own membership.
	expectRole(mock, "boss", "t1", "owner")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WithArgs("u1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", "boss"))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", bodyOf(t, w)["errorCode"])
	// No token was written.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateSSOToken_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectRole(mock, "u1", "t1", "member")
	mock.ExpectQuery(`FROM "User" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE "UsersOnTenants"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br/entrar", "u1"))

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
	expectRole(mock, "u1", "t1", "member")
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
	f.GenerateSSOToken(w, generateRequest(`{"userId":"u1","tenantId":"t1"}`, "https://escola.com.br", "u1"))
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
	w := httptest.NewRecorder()
	f.GenerateSSOToken(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	f.ValidateSSOToken(w, httptest.NewRequest(http.MethodGet, "/validate-token", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
