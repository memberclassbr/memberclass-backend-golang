package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

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
	mock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(hex.EncodeToString(sum[:])).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("t1", "Cliente"))

	m := NewAuthExternalMiddleware(db, fakeLogger{})

	var gotTenantID string
	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenant := constants.GetTenantFromContext(r.Context()); tenant != nil {
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

		m := NewAuthExternalMiddleware(db, fakeLogger{})
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

		mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(sql.ErrNoRows)

		m := NewAuthExternalMiddleware(db, fakeLogger{})
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

	mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(assert.AnError)

	m := NewAuthExternalMiddleware(db, fakeLogger{})
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

// ---------- NextAuth session cookie ----------

func TestAuthMiddleware_RejectsMissingCookie(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cfg := &config.Config{Auth: config.Auth{NextAuthSecret: "secret"}}
	m := NewAuthMiddleware(db, cfg, fakeLogger{})
	reached := false

	w := httptest.NewRecorder()
	m.Authenticate(okHandler(&reached)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, reached)
}

func TestAuthMiddleware_RejectsUndecryptableCookie(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cfg := &config.Config{Auth: config.Auth{NextAuthSecret: "secret"}}
	m := NewAuthMiddleware(db, cfg, fakeLogger{})
	reached := false

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "next-auth.session-token", Value: "garbage"})
	w := httptest.NewRecorder()
	m.Authenticate(okHandler(&reached)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, reached)
}

// The key derivation must depend on the configured secret; two different
// secrets must not derive the same key, or a rotated secret would keep
// accepting old cookies.
func TestAuthMiddleware_KeyDerivationDependsOnSecret(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	a := NewAuthMiddleware(db, &config.Config{Auth: config.Auth{NextAuthSecret: "secret-a"}}, fakeLogger{})
	b := NewAuthMiddleware(db, &config.Config{Auth: config.Auth{NextAuthSecret: "secret-b"}}, fakeLogger{})

	keyA, err := a.deriveEncryptionKey()
	require.NoError(t, err)
	keyB, err := b.deriveEncryptionKey()
	require.NoError(t, err)

	assert.Len(t, keyA, 32)
	assert.NotEqual(t, keyA, keyB)
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

func TestBearer_AcceptsValidToken(t *testing.T) {
	const secret = "shared-with-nextjs"
	cfg := &config.Config{Auth: config.Auth{NextAuthSecret: secret}}
	m := NewBearerMiddleware(cfg, fakeLogger{})

	token := signJWT(t, secret, map[string]any{
		"sub":   "u1",
		"email": "admin@example.com",
		"role":  "owner",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	var got *AuthUser
	handler := m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetAuthUser(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "u1", got.UserID)
	assert.Equal(t, "owner", got.Role)
}

func TestBearer_Rejects(t *testing.T) {
	const secret = "shared-with-nextjs"
	cfg := &config.Config{Auth: config.Auth{NextAuthSecret: secret}}
	m := NewBearerMiddleware(cfg, fakeLogger{})

	cases := map[string]string{
		"no header":         "",
		"not bearer":        "Basic abc",
		"garbage token":     "Bearer not-a-jwt",
		"wrong signature":   "Bearer " + signJWT(t, "other-secret", map[string]any{"sub": "u1", "exp": time.Now().Add(time.Hour).Unix()}),
		"expired token":     "Bearer " + signJWT(t, secret, map[string]any{"sub": "u1", "exp": time.Now().Add(-time.Hour).Unix()}),
		"none-alg attempt":  "Bearer " + signJWT(t, "", map[string]any{"sub": "u1", "exp": time.Now().Add(time.Hour).Unix()}),
		"empty bearer body": "Bearer ",
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			reached := false
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			w := httptest.NewRecorder()
			m.RequireAuth(okHandler(&reached)).ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.False(t, reached)
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
