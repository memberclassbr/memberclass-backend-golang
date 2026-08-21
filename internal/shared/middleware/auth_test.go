package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// fakeCache stands in for Redis. Only Exists matters here — it is the one call
// the revocation denylist makes.
type fakeCache struct {
	exists    map[string]bool
	existsErr error
}

func newFakeCache() *fakeCache { return &fakeCache{exists: map[string]bool{}} }

func (c *fakeCache) Exists(_ context.Context, key string) (bool, error) {
	if c.existsErr != nil {
		return false, c.existsErr
	}
	return c.exists[key], nil
}

func (c *fakeCache) Get(context.Context, string) (string, error)              { return "", nil }
func (c *fakeCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (c *fakeCache) Increment(context.Context, string, int64) (int64, error)  { return 0, nil }
func (c *fakeCache) Delete(context.Context, string) error                     { return nil }
func (c *fakeCache) TTL(context.Context, string) (time.Duration, error)       { return 0, nil }
func (c *fakeCache) Close() error                                             { return nil }

var _ cache.Cache = (*fakeCache)(nil)

// okHandler records whether the middleware let the request through.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

// ---------- mc-api-key ----------

func TestAuthExternal_ValidKeyResolvesTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	const plaintext = "tenant-api-key"
	sum := sha256.Sum256([]byte(plaintext))

	// The header carries the plaintext; only its hash is ever queried.
	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WithArgs(hex.EncodeToString(sum[:])).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("k1", "t1", "Cliente"))

	m := NewAuthExternalMiddleware(db, fakeLogger{}, nil)

	var gotTenantID string
	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenant := tenant.FromContext(r.Context()); tenant != nil {
			gotTenantID = tenant.ID
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("mc-api-key", plaintext)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "t1", gotTenantID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthExternal_RejectsMissingAndUnknownKeys(t *testing.T) {
	t.Run("no header", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		m := NewAuthExternalMiddleware(db, fakeLogger{}, nil)
		reached := false

		w := httptest.NewRecorder()
		m.Authenticate(okHandler(&reached)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, reached)
		// No query ran: a missing header is rejected before the lookup.
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown key", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(`FROM "TenantApiKey"`).WillReturnError(sql.ErrNoRows)

		m := NewAuthExternalMiddleware(db, fakeLogger{}, nil)
		reached := false

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("mc-api-key", "nope")
		w := httptest.NewRecorder()
		m.Authenticate(okHandler(&reached)).ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, reached)
	})
}

// A lookup failure and an unknown key must be indistinguishable to the caller,
// so the endpoint cannot be used to probe which keys exist.
func TestAuthExternal_DatabaseErrorLooksLikeAnInvalidKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).WillReturnError(assert.AnError)

	m := NewAuthExternalMiddleware(db, fakeLogger{}, nil)
	reached := false

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("mc-api-key", "some-key")
	w := httptest.NewRecorder()
	m.Authenticate(okHandler(&reached)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, reached)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_API_KEY", body["errorCode"])
}

// ---------- Bearer JWT ----------

// signJWT builds a compact HS256 token the middleware should accept.
func signJWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))

	return signingInput + "." + enc.EncodeToString(mac.Sum(nil))
}

// bearerSecret is long enough to pass the floor config.Load enforces, so a
// test never accidentally exercises a configuration production cannot have.
const bearerSecret = "go-api-secret-at-least-32-bytes-long"

// validClaims is the full claim set the frontend mints today. Tests that check
// a rejection start from this and remove or corrupt exactly one thing.
func validClaims() map[string]any {
	return map[string]any{
		"sub":      "u1",
		"email":    "admin@example.com",
		"tenantId": "t1",
		"role":     "owner",
		"aud":      Audience,
		"jti":      "token-1",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
}

func newBearer(c cache.Cache) *BearerMiddleware {
	cfg := &config.Config{Auth: config.Auth{GoAPIJWTSecret: bearerSecret}}
	return NewBearerMiddleware(cfg, c, fakeLogger{})
}

// serveBearer runs one request through the middleware and reports the recorder
// plus the identity the handler saw (nil when it never ran).
func serveBearer(m *BearerMiddleware, header string) (*httptest.ResponseRecorder, *AuthUser) {
	var got *AuthUser
	handler := m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetAuthUser(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w, got
}

func TestBearer_AcceptsValidToken(t *testing.T) {
	m := newBearer(nil)

	w, got := serveBearer(m, "Bearer "+signJWT(t, bearerSecret, validClaims()))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "u1", got.UserID)
	assert.Equal(t, "owner", got.Role)
	assert.Equal(t, "t1", got.TenantID)
	assert.Equal(t, "token-1", got.ID)
}

// The tenant is the token's whole scope, so it has to survive verification
// intact — everything downstream reads it instead of the request body.
func TestBearer_CarriesTheTenantClaim(t *testing.T) {
	m := newBearer(nil)

	claims := validClaims()
	claims["tenantId"] = "tenant-from-the-token"

	_, got := serveBearer(m, "Bearer "+signJWT(t, bearerSecret, claims))

	require.NotNil(t, got)
	assert.Equal(t, "tenant-from-the-token", got.TenantID)
}

// RFC 7519 allows `aud` to be a string or an array; Node's jsonwebtoken emits
// whichever fits, so both have to verify.
func TestBearer_AcceptsAudienceAsStringOrArray(t *testing.T) {
	m := newBearer(nil)

	for name, aud := range map[string]any{
		"string":                   Audience,
		"array":                    []string{Audience},
		"array with other entries": []string{"some-other-service", Audience},
	} {
		t.Run(name, func(t *testing.T) {
			claims := validClaims()
			claims["aud"] = aud

			w, _ := serveBearer(m, "Bearer "+signJWT(t, bearerSecret, claims))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestBearer_Rejects(t *testing.T) {
	m := newBearer(nil)

	// Each of these is the valid claim set minus (or with a corrupted) one
	// thing. Verification defaults nothing: a claim that is merely absent is
	// the shape a forged or stale token takes.
	withoutClaim := func(key string) string {
		claims := validClaims()
		delete(claims, key)
		return "Bearer " + signJWT(t, bearerSecret, claims)
	}
	withClaim := func(key string, value any) string {
		claims := validClaims()
		claims[key] = value
		return "Bearer " + signJWT(t, bearerSecret, claims)
	}

	cases := map[string]string{
		"no header":            "",
		"not bearer":           "Basic abc",
		"garbage token":        "Bearer not-a-jwt",
		"empty bearer body":    "Bearer ",
		"wrong signature":      "Bearer " + signJWT(t, "another-secret-at-least-32-bytes!!", validClaims()),
		"none-alg attempt":     "Bearer " + signJWT(t, "", validClaims()),
		"expired token":        withClaim("exp", time.Now().Add(-time.Hour).Unix()),
		"no exp":               withoutClaim("exp"),
		"no sub":               withoutClaim("sub"),
		"no tenantId":          withoutClaim("tenantId"),
		"empty tenantId":       withClaim("tenantId", ""),
		"no aud":               withoutClaim("aud"),
		"another service aud":  withClaim("aud", "some-other-service"),
		"aud array without us": withClaim("aud", []string{"some-other-service"}),
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			w, got := serveBearer(m, header)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Nil(t, got)
		})
	}
}

// ---------- Bearer revocation ----------

// A jti the frontend published to the denylist is refused even though the
// signature and the expiry are both fine.
func TestBearer_RejectsRevokedToken(t *testing.T) {
	c := newFakeCache()
	c.exists[RevocationKey("token-1")] = true

	w, got := serveBearer(newBearer(c), "Bearer "+signJWT(t, bearerSecret, validClaims()))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, got)
}

// A token whose jti is not on the list passes, and one that carries no jti at
// all is not rejected for lacking it — the claim is the frontend's to emit, and
// refusing tokens without it would break the first deploy that lands out of
// step.
func TestBearer_AllowsUnrevokedAndJTILessTokens(t *testing.T) {
	c := newFakeCache()
	c.exists[RevocationKey("some-other-token")] = true

	t.Run("different jti", func(t *testing.T) {
		w, _ := serveBearer(newBearer(c), "Bearer "+signJWT(t, bearerSecret, validClaims()))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("no jti at all", func(t *testing.T) {
		claims := validClaims()
		delete(claims, "jti")
		w, _ := serveBearer(newBearer(c), "Bearer "+signJWT(t, bearerSecret, claims))
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// Redis being down must not take every admin route with it. The window the
// denylist shortens is already bounded by exp, and role changes never depend on
// this path — tenantrole re-reads the row on every request.
func TestBearer_RevocationLookupFailureFailsOpen(t *testing.T) {
	c := newFakeCache()
	c.existsErr = errors.New("redis unreachable")

	w, got := serveBearer(newBearer(c), "Bearer "+signJWT(t, bearerSecret, validClaims()))

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "u1", got.UserID)
}

// ---------- Migrating off NEXTAUTH_SECRET ----------

// sessionSecret stands in for NEXTAUTH_SECRET, which also derives the session
// cookie's key. Getting a go-token off it is the whole point of the migration.
const sessionSecret = "nextauth-session-secret-32-bytes-ok"

// State 1 — GO_API_JWT_SECRET unset. The frontend falls back to signing with
// NEXTAUTH_SECRET when its own is unset, so a backend that refused this would
// reject every token from a frontend that has not had the env added yet.
func TestBearer_FallsBackToTheSessionSecret(t *testing.T) {
	cfg := &config.Config{Auth: config.Auth{
		NextAuthSecret:       sessionSecret,
		AllowLegacyJWTSecret: true,
	}}
	m := NewBearerMiddleware(cfg, nil, fakeLogger{})

	w, got := serveBearer(m, "Bearer "+signJWT(t, sessionSecret, validClaims()))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "u1", got.UserID)
}

// State 2 — both set. Neither side has to deploy first: whichever key the
// frontend is signing with today keeps working while the env catches up.
func TestBearer_AcceptsEitherSecretDuringTheMigration(t *testing.T) {
	cfg := &config.Config{Auth: config.Auth{
		NextAuthSecret:       sessionSecret,
		GoAPIJWTSecret:       bearerSecret,
		AllowLegacyJWTSecret: true,
	}}
	m := NewBearerMiddleware(cfg, nil, fakeLogger{})

	for name, secret := range map[string]string{
		"the dedicated secret": bearerSecret,
		"the session secret":   sessionSecret,
	} {
		t.Run(name, func(t *testing.T) {
			w, _ := serveBearer(m, "Bearer "+signJWT(t, secret, validClaims()))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// State 3 — the fallback is closed. This is the state that actually buys
// something: until a deployment reaches it, a leaked go-token key is still the
// key the session cookie derives from.
func TestBearer_StopsAcceptingTheSessionSecretOnceTheFallbackIsOff(t *testing.T) {
	cfg := &config.Config{Auth: config.Auth{
		NextAuthSecret:       sessionSecret,
		GoAPIJWTSecret:       bearerSecret,
		AllowLegacyJWTSecret: false,
	}}
	m := NewBearerMiddleware(cfg, nil, fakeLogger{})

	t.Run("the session secret is refused", func(t *testing.T) {
		w, _ := serveBearer(m, "Bearer "+signJWT(t, sessionSecret, validClaims()))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
	t.Run("the dedicated secret still verifies", func(t *testing.T) {
		w, _ := serveBearer(m, "Bearer "+signJWT(t, bearerSecret, validClaims()))
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// A third key verifies in no state at all — the fallback widens what is
// accepted by exactly one known secret, not by anything that happens to be
// configured.
func TestBearer_RejectsAnUnrelatedSecretInEveryState(t *testing.T) {
	const attacker = "attacker-controlled-secret-32-byte"

	for name, auth := range map[string]config.Auth{
		"fallback only":  {NextAuthSecret: sessionSecret, AllowLegacyJWTSecret: true},
		"both":           {NextAuthSecret: sessionSecret, GoAPIJWTSecret: bearerSecret, AllowLegacyJWTSecret: true},
		"dedicated only": {NextAuthSecret: sessionSecret, GoAPIJWTSecret: bearerSecret},
	} {
		t.Run(name, func(t *testing.T) {
			m := NewBearerMiddleware(&config.Config{Auth: auth}, nil, fakeLogger{})
			w, _ := serveBearer(m, "Bearer "+signJWT(t, attacker, validClaims()))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// GetAuthUser must return nil for a request that never went through the
// middleware, so a handler cannot mistake an unauthenticated request for an
// authenticated one.
func TestGetAuthUser_NilWithoutMiddleware(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Nil(t, GetAuthUser(req.Context()))
}

// ---------- x-api-key ----------

func TestAuthExternal_AcceptsTheCurrentHeader(t *testing.T) {
	plaintext := "tenant-key"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sum := sha256.Sum256([]byte(plaintext))
	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WithArgs(hex.EncodeToString(sum[:])).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("k1", "t1", "Acme"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("x-api-key", plaintext)

	rec := httptest.NewRecorder()
	var reached bool
	NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
	).ServeHTTP(rec, req)

	assert.True(t, reached, "x-api-key is the header we are asking callers to move to")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// The legacy name stays accepted and unadvertised. Dropping it would log out
// every integration in a single deploy.
func TestAuthExternal_StillAcceptsTheLegacyHeader(t *testing.T) {
	plaintext := "tenant-key"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sum := sha256.Sum256([]byte(plaintext))
	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WithArgs(hex.EncodeToString(sum[:])).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("k1", "t1", "Acme"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("mc-api-key", plaintext)

	rec := httptest.NewRecorder()
	var reached bool
	NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
	).ServeHTTP(rec, req)

	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// During a migration a caller may send both. They carry the same key, so the
// request is served rather than rejected for the ambiguity.
func TestAuthExternal_PrefersTheCurrentHeaderWhenBothAreSent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sum := sha256.Sum256([]byte("current"))
	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WithArgs(hex.EncodeToString(sum[:])).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("k1", "t1", "Acme"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("x-api-key", "current")
	req.Header.Set("mc-api-key", "legacy")

	rec := httptest.NewRecorder()
	NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NoError(t, mock.ExpectationsWereMet(), "the lookup must use x-api-key")
}

func TestAuthExternal_RejectsWhenNeitherHeaderIsPresent(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rec := httptest.NewRecorder()
	NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("handler must not run without a credential")
		}),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- expiry, error codes and usage ----------

// An expired key is refused by the same query that authenticates, so the
// middleware never sees it. What matters here is that the answer is byte for
// byte the answer an unknown key gets: telling the two apart would tell someone
// guessing that a value was once real.
func TestAuthExternal_ExpiredKeyAnswersLikeAnUnknownOne(t *testing.T) {
	body := func(t *testing.T, err error) map[string]any {
		t.Helper()

		db, mock, dbErr := sqlmock.New()
		require.NoError(t, dbErr)
		defer db.Close()

		mock.ExpectQuery(`FROM "TenantApiKey"`).WillReturnError(err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("x-api-key", "some-key")
		rec := httptest.NewRecorder()

		NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
			http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler must not run")
			}),
		).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
		return decoded
	}

	// The expiry lives in the WHERE clause, so an expired key is exactly a row
	// that did not come back.
	expired := body(t, sql.ErrNoRows)
	unknown := body(t, sql.ErrNoRows)

	assert.Equal(t, "INVALID_API_KEY", expired["errorCode"])
	assert.Equal(t, expired, unknown)
}

// A missing header and a wrong key are different errorCodes. The caller that
// sent nothing learns nothing it did not already know.
func TestAuthExternal_MissingHeaderHasItsOwnErrorCode(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rec := httptest.NewRecorder()
	NewAuthExternalMiddleware(db, fakeLogger{}, nil).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("handler must not run without a credential")
		}),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
	assert.Equal(t, "MISSING_API_KEY", decoded["errorCode"])
	assert.Equal(t, false, decoded["ok"])
}

// recordedUsage is one call to the recorder.
type recordedUsage struct {
	apiKeyID string
	endpoint string
	status   int
}

type fakeUsageRecorder struct{ calls []recordedUsage }

func (f *fakeUsageRecorder) Record(_ context.Context, apiKeyID, endpoint string, status int) {
	f.calls = append(f.calls, recordedUsage{apiKeyID, endpoint, status})
}

// authenticatedRequest runs one request through a chi router with the
// middleware mounted, which is the only way the route pattern resolves.
func authenticatedRequest(t *testing.T, pattern, path string, handler http.HandlerFunc) *fakeUsageRecorder {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("key-1", "t1", "Acme"))

	usage := &fakeUsageRecorder{}
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(NewAuthExternalMiddleware(db, fakeLogger{}, usage).Authenticate)
		r.Get(pattern, handler)
	})

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("x-api-key", "some-key")
	router.ServeHTTP(httptest.NewRecorder(), req)

	return usage
}

func TestAuthExternal_RecordsUsageWithTheRoutePattern(t *testing.T) {
	usage := authenticatedRequest(t, "/comments/{commentId}", "/api/v1/comments/abc123",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	require.Len(t, usage.calls, 1)
	// The id in the path never reaches the row: one series per endpoint, not
	// one per comment.
	assert.Equal(t, recordedUsage{"key-1", "comments/:commentId", http.StatusOK}, usage.calls[0])
}

// A handler that writes nothing still answered 200, and that is what the panel
// must show.
func TestAuthExternal_RecordsTwoHundredForASilentHandler(t *testing.T) {
	usage := authenticatedRequest(t, "/user/informations", "/api/v1/user/informations",
		func(http.ResponseWriter, *http.Request) {})

	require.Len(t, usage.calls, 1)
	assert.Equal(t, http.StatusOK, usage.calls[0].status)
}

// The status is read after the handler ran, which is what lets a failure be
// counted as one. A 429 from the rate limiter mounted below arrives here the
// same way.
func TestAuthExternal_RecordsTheFailureStatus(t *testing.T) {
	usage := authenticatedRequest(t, "/user/informations", "/api/v1/user/informations",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })

	require.Len(t, usage.calls, 1)
	assert.Equal(t, http.StatusTooManyRequests, usage.calls[0].status)
}

// Without a router there is no pattern, only a raw path — and a row per path is
// the cardinality the pattern exists to avoid.
func TestAuthExternal_RecordsNothingWithoutARoutePattern(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id", "name"}).AddRow("key-1", "t1", "Acme"))

	usage := &fakeUsageRecorder{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/comments/abc123", nil)
	req.Header.Set("x-api-key", "some-key")

	NewAuthExternalMiddleware(db, fakeLogger{}, usage).Authenticate(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }),
	).ServeHTTP(httptest.NewRecorder(), req)

	assert.Empty(t, usage.calls)
}

// A request that never authenticated has no key to charge.
func TestAuthExternal_RecordsNothingForARejectedRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).WillReturnError(sql.ErrNoRows)

	usage := &fakeUsageRecorder{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("x-api-key", "nope")

	NewAuthExternalMiddleware(db, fakeLogger{}, usage).Authenticate(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler must not run") }),
	).ServeHTTP(httptest.NewRecorder(), req)

	assert.Empty(t, usage.calls)
}
