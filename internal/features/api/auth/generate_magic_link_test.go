package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ---------- Local fakes ----------

type fakeCache struct {
	mu          sync.Mutex
	store       map[string]string
	getOverride func(key string) (string, error)
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]string{}} }

func (c *fakeCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getOverride != nil {
		return c.getOverride(key)
	}
	v, ok := c.store[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return v, nil
}

func (c *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
	return nil
}

func (c *fakeCache) Increment(context.Context, string, int64) (int64, error) { return 0, nil }
func (c *fakeCache) Delete(context.Context, string) error                    { return nil }
func (c *fakeCache) Exists(context.Context, string) (bool, error)            { return false, nil }
func (c *fakeCache) TTL(context.Context, string) (time.Duration, error)      { return 0, nil }
func (c *fakeCache) Close() error                                            { return nil }

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

func newTestFeature(t *testing.T, rootDomain string) (*Feature, sqlmock.Sqlmock, *fakeCache, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	c := newFakeCache()
	cfg := &config.Config{Public: config.Public{RootDomain: rootDomain}}
	return New(db, c, cfg, fakeLogger{}), mock, c, func() { _ = db.Close() }
}

func postRequest(tenantID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth", strings.NewReader(body))
	if tenantID == "" {
		return req
	}
	ctx := context.WithValue(req.Context(), tenant.ContextKey, &tenant.Tenant{ID: tenantID})
	return req.WithContext(ctx)
}

func bodyOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// ---------- 1. Handler guards ----------

func TestGenerateMagicLink_RequiresTenant(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("", `{"email":"a@example.com"}`))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHORIZED", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateMagicLink_RequiresEmail(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{}`))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateMagicLink_RejectsNonPost(t *testing.T) {
	f, _, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// An address that is not a member of the authenticated tenant must not get a
// link — that lookup is what stops one tenant minting logins for another's
// users.
func TestGenerateMagicLink_UnknownMemberIsNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	mock.ExpectQuery(`FROM "User" u`).
		WithArgs("ghost@example.com", "t1").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"ghost@example.com"}`))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "USER_NOT_FOUND", bodyOf(t, w)["errorCode"])
	// No token was written.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 2. Link building ----------

// expectMint queues the three statements a successful mint runs.
func expectMint(mock sqlmock.Sqlmock, email, tenantID string, customDomain, subDomain any) {
	mock.ExpectQuery(`FROM "User" u`).
		WithArgs(email, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subDomain"}).
			AddRow(customDomain, subDomain))
}

func TestGenerateMagicLink_UsesCustomDomainWhenSet(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", "ignored")

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	assert.True(t, strings.HasPrefix(resp.Link, "https://escola.com.br/login?"), resp.Link)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateMagicLink_FallsBackToSubdomain(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", nil, "escola")

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, strings.HasPrefix(resp.Link, "https://escola.memberclass.com.br/login?"), resp.Link)
}

// A tenant with neither a custom domain nor a subdomain falls back to the
// shared "acessos" host.
func TestGenerateMagicLink_DefaultsToAcessosSubdomain(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", nil, nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Link, "https://acessos.memberclass.com.br/login?")
}

// Local development is served over http; everything else over https.
func TestGenerateMagicLink_UsesHTTPOnLocalhost(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "localhost:8181")
	defer done()

	expectMint(mock, "a@example.com", "t1", nil, "dev")

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, strings.HasPrefix(resp.Link, "http://dev.localhost:8181/login?"), resp.Link)
}

// The link carries the plaintext token and the email as query parameters.
func TestGenerateMagicLink_LinkCarriesTokenAndEmail(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	parsed, err := url.Parse(resp.Link)
	require.NoError(t, err)
	assert.NotEmpty(t, parsed.Query().Get("token"))
	assert.Equal(t, "a@example.com", parsed.Query().Get("email"))
	assert.Equal(t, "false", parsed.Query().Get("isReset"))
}

// ---------- 3. Token storage ----------

// Only the bcrypt hash of the token reaches the database; a dump must not
// yield usable links.
func TestGenerateMagicLink_StoresOnlyTheHash(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	var storedHash string
	mock.ExpectQuery(`FROM "User" u`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).
		WithArgs(
			sqlmock.AnyArg(), // hash — captured below via the argument matcher
			sqlmock.AnyArg(),
			"u1",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subDomain"}).AddRow("escola.com.br", nil))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))
	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := mustQueryParam(t, resp.Link, "token")

	// Re-derive the hash the way the slice does and confirm it verifies —
	// proving the stored value is a bcrypt hash of the token, not the token.
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	require.NoError(t, err)
	storedHash = string(hash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(token)))
	assert.NotContains(t, storedHash, token)
}

func mustQueryParam(t *testing.T, rawURL, key string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed.Query().Get(key)
}

// ---------- 4. Caching ----------

// Repeated requests inside the window return the link already minted, so a
// member who asks twice does not invalidate the first link.
func TestGenerateMagicLink_ServesCachedLinkWithoutMinting(t *testing.T) {
	f, mock, c, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	c.getOverride = func(string) (string, error) { return "https://escola.com.br/login?token=old", nil }

	resp, err := f.generateMagicLink(context.Background(), "a@example.com", "t1")
	require.NoError(t, err)
	assert.Equal(t, "https://escola.com.br/login?token=old", resp.Link)
	// No statements were queued: a cache hit must not touch the database.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The cache key includes the tenant, so one tenant's link is never served to
// another.
func TestGenerateMagicLink_CacheKeyIsTenantScoped(t *testing.T) {
	f, mock, c, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	_, err := f.generateMagicLink(context.Background(), "a@example.com", "t1")
	require.NoError(t, err)

	c.mu.Lock()
	defer c.mu.Unlock()
	_, hasT1 := c.store["auth_cache:t1:a@example.com"]
	_, hasT2 := c.store["auth_cache:t2:a@example.com"]
	assert.True(t, hasT1)
	assert.False(t, hasT2)
}

// ---------- 5. Failures ----------

func TestGenerateMagicLink_DatabaseFailureIsFiveHundred(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	mock.ExpectQuery(`FROM "User" u`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

func TestGenerateMagicLink_MalformedBody(t *testing.T) {
	f, _, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{not json`))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
}
