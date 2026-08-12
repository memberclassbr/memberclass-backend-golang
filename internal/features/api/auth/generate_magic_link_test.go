package auth

import (
	"context"
	"database/sql"
	"database/sql/driver"
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
	"github.com/lib/pq"
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

func newTestFeature(t *testing.T, publicDomain string) (*Feature, sqlmock.Sqlmock, *fakeCache, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	c := newFakeCache()
	cfg := &config.Config{Public: config.Public{DomainURL: publicDomain}}
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

// expectMint queues the four statements a successful mint runs.
func expectMint(mock sqlmock.Sqlmock, email, tenantID string, customDomain, subDomain any) {
	mock.ExpectQuery(`FROM "User" u`).
		WithArgs(email, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The column list here is pinned for the same reason as the Tenant one
	// below: Prisma's casing is not guessable, and "shortCode"/"userId"/
	// "tenantId" are all camelCase while method and token are not.
	mock.ExpectExec(`INSERT INTO "MagicToken" \(id, token, "shortCode", "userId", "tenantId", method, expires, "createdAt"\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The column list is pinned on purpose. Prisma names the Tenant columns
	// "customDomain" (camelCase) and subdomain (not), and a quoted "subDomain"
	// is a runtime-only failure: sqlmock does not know the schema, so a casing
	// slip used to reach production as a 500. Matching the identifiers here is
	// the only place a unit test can catch it.
	mock.ExpectQuery(`SELECT "customDomain", subdomain FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subdomain"}).
			AddRow(customDomain, subDomain))
}

// shortCodeOf pulls the short code out of a `/api/auth/magic/<code>` link.
func shortCodeOf(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	prefix := "/api/auth/magic/"
	require.True(t, strings.HasPrefix(parsed.Path, prefix), "unexpected path %q", parsed.Path)
	return strings.TrimPrefix(parsed.Path, prefix)
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
	assert.True(t, strings.HasPrefix(resp.Link, "https://escola.com.br/api/auth/magic/"), resp.Link)
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
	assert.True(t, strings.HasPrefix(resp.Link, "https://escola.memberclass.com.br/api/auth/magic/"), resp.Link)
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
	assert.Contains(t, resp.Link, "https://acessos.memberclass.com.br/api/auth/magic/")
}

// Local development is served over http; everything else over https.
func TestGenerateMagicLink_UsesHTTPOnLocalhost(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "localhost:3000")
	defer done()

	expectMint(mock, "a@example.com", "t1", nil, "dev")

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, strings.HasPrefix(resp.Link, "http://dev.localhost:3000/api/auth/magic/"), resp.Link)
}

// The point of the short-code format: the URL identifies the row and nothing
// else. A login link ends up in mail archives, proxy logs and screenshots, so
// neither the member's address nor the token itself may travel in it.
func TestGenerateMagicLink_LinkExposesNeitherTokenNorEmail(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotContains(t, resp.Link, "a@example.com")
	assert.NotContains(t, resp.Link, "a%40example.com")
	assert.NotContains(t, resp.Link, "token=")
	assert.NotContains(t, resp.Link, "isReset")
	assert.Len(t, shortCodeOf(t, resp.Link), 12)
}

// ---------- 2b. Redirect path ----------

func TestGenerateMagicLink_CarriesRedirectPath(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com","redirectPath":"/curso/123/aula/456"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "/curso/123/aula/456", mustQueryParam(t, resp.Link, "redirect"))
	// The destination is the only query parameter; the code stays in the path.
	assert.NotEmpty(t, shortCodeOf(t, resp.Link))
}

// Omitting the field keeps the link exactly as it was before this parameter
// existed — no empty `redirect=` for the frontend to special-case.
func TestGenerateMagicLink_OmitsRedirectWhenAbsent(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotContains(t, resp.Link, "redirect=")
}

// A query string or a fragment on the destination survives the round trip:
// they are part of the path the frontend has to restore.
func TestGenerateMagicLink_RedirectKeepsQueryAndFragment(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com","redirectPath":"/busca?q=vendas&page=2#topo"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "/busca?q=vendas&page=2#topo", mustQueryParam(t, resp.Link, "redirect"))
}

// The open-redirect guard. Each of these is a value a browser would resolve to
// an origin other than the tenant's, which would turn the magic link into a
// way to land a freshly authenticated member on an attacker's page.
func TestGenerateMagicLink_RejectsOffSiteRedirects(t *testing.T) {
	hostile := map[string]string{
		"absolute URL":      "https://evil.com/steal",
		"protocol relative": "//evil.com/steal",
		"backslash host":    `/\evil.com/steal`,
		"javascript scheme": "javascript:alert(1)",
		"data scheme":       "data:text/html,<script>alert(1)</script>",
		"no leading slash":  "evil.com/steal",
		"header break":      "/curso\r\nLocation: https://evil.com",
	}

	for name, path := range hostile {
		t.Run(name, func(t *testing.T) {
			f, mock, _, done := newTestFeature(t, "memberclass.com.br")
			defer done()

			body, err := json.Marshal(authRequest{Email: "a@example.com", RedirectPath: path})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			f.GenerateMagicLink(w, postRequest("t1", string(body)))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
			// Rejected before any token was minted or stored.
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGenerateMagicLink_RejectsOverlongRedirect(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	body, err := json.Marshal(authRequest{
		Email:        "a@example.com",
		RedirectPath: "/" + strings.Repeat("a", maxRedirectPathLen),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", string(body)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Two requests inside the cache window that differ only in destination share
// the token already minted. Minting a second one would invalidate the first,
// since "User"."magicToken" holds a single value per member.
func TestGenerateMagicLink_RedirectVariesWithoutReminting(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	expectMint(mock, "a@example.com", "t1", "escola.com.br", nil)

	first, err := f.generateMagicLink(context.Background(), "a@example.com", "/curso/1", "t1")
	require.NoError(t, err)

	// No further statements are queued: a second destination must be served
	// from the cache.
	second, err := f.generateMagicLink(context.Background(), "a@example.com", "/curso/2", "t1")
	require.NoError(t, err)

	assert.Equal(t, "/curso/1", mustQueryParam(t, first.Link, "redirect"))
	assert.Equal(t, "/curso/2", mustQueryParam(t, second.Link, "redirect"))
	assert.Equal(t,
		shortCodeOf(t, first.Link),
		shortCodeOf(t, second.Link),
		"both links must resolve to the same MagicToken row")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 3. Token storage ----------

// The row the frontend redeems: its "shortCode" is what the link carries, its
// "token" is a sha256 digest, and its "method" names this endpoint so the audit
// trail can tell an API-minted link from one the panel produced.
func TestGenerateMagicLink_MintsTheMagicTokenRow(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	var storedToken, storedShortCode, storedMethod string
	var storedUserID, storedTenantID string

	mock.ExpectQuery(`FROM "User" u`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WithArgs(
			sqlmock.AnyArg(), // id (CUID)
			capture(&storedToken),
			capture(&storedShortCode),
			capture(&storedUserID),
			capture(&storedTenantID),
			capture(&storedMethod),
			sqlmock.AnyArg(), // expires
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "customDomain", subdomain FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subdomain"}).AddRow("escola.com.br", nil))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))
	require.Equal(t, http.StatusOK, w.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, storedShortCode, shortCodeOf(t, resp.Link))
	assert.Equal(t, "u1", storedUserID)
	assert.Equal(t, "t1", storedTenantID)
	assert.Equal(t, magicTokenMethod, storedMethod)
	// sha256 hex, not bcrypt: the frontend compares digests.
	assert.Len(t, storedToken, 64)
	assert.NotContains(t, storedToken, "$2a$")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A collision on the unique "shortCode" index is regenerated rather than
// surfaced to the caller.
func TestGenerateMagicLink_RetriesOnShortCodeCollision(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	mock.ExpectQuery(`FROM "User" u`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "MagicToken_shortCode_key"})
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "customDomain", subdomain FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subdomain"}).AddRow("escola.com.br", nil))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// If the row cannot be minted the request fails. Degrading to the old
// `/login?token=&email=` link would put the address back in the URL, silently.
func TestGenerateMagicLink_FailsWhenTheRowCannotBeMinted(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	mock.ExpectQuery(`FROM "User" u`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// The legacy "User"."magicToken" column keeps its bcrypt hash: the frontend
// still honours it on the old /login route, and a dump must not yield a usable
// token.
func TestGenerateMagicLink_StoresOnlyTheHashOnTheLegacyColumn(t *testing.T) {
	f, mock, _, done := newTestFeature(t, "memberclass.com.br")
	defer done()

	var legacyHash, digest string

	mock.ExpectQuery(`FROM "User" u`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectExec(`UPDATE "User"`).
		WithArgs(capture(&legacyHash), sqlmock.AnyArg(), "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "MagicToken"`).
		WithArgs(
			sqlmock.AnyArg(),
			capture(&digest),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "customDomain", subdomain FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"customDomain", "subdomain"}).AddRow("escola.com.br", nil))

	w := httptest.NewRecorder()
	f.GenerateMagicLink(w, postRequest("t1", `{"email":"a@example.com"}`))
	require.Equal(t, http.StatusOK, w.Code)

	// The raw token never leaves the process, so the assertion runs the other
	// way: whatever the legacy column holds must be a bcrypt hash, and the two
	// columns must not hold the same value.
	assert.True(t, strings.HasPrefix(legacyHash, "$2"), "not a bcrypt hash: %q", legacyHash)
	assert.NotEqual(t, legacyHash, digest)
	assert.Error(t, bcrypt.CompareHashAndPassword([]byte(legacyHash), []byte(digest)))
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

	resp, err := f.generateMagicLink(context.Background(), "a@example.com", "", "t1")
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

	_, err := f.generateMagicLink(context.Background(), "a@example.com", "", "t1")
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
