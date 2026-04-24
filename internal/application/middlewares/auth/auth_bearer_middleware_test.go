package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fake logger (no mockery — no strict expectations) ----------

type silentLogger struct{}

func (silentLogger) Debug(string, ...any) {}
func (silentLogger) Info(string, ...any)  {}
func (silentLogger) Warn(string, ...any)  {}
func (silentLogger) Error(string, ...any) {}

// ---------- Helpers ----------

func newBearer(t *testing.T, secret string) *BearerMiddleware {
	t.Helper()
	return &BearerMiddleware{
		logger: silentLogger{},
		secret: []byte(secret),
		now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

// signJWT builds a compact HS256 JWT over `claims` using `secret`, matching
// Node's `jsonwebtoken` behavior (no minimum key length). Not using a lib —
// keeps the test dependency-free and also the shape honest since this
// mirrors exactly what the middleware's verifyHS256 accepts.
func signJWT(t *testing.T, secret string, claims any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return h + "." + p + "." + sig
}

func runMiddleware(t *testing.T, m *BearerMiddleware, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	nextCalled := false
	var gotUser *AuthUser
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		gotUser = GetAuthUser(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	m.RequireAuth(next).ServeHTTP(w, req)
	// Expose the side-effects via header bag so individual tests can read.
	if nextCalled {
		w.Header().Set("X-Test-NextCalled", "true")
		if gotUser != nil {
			w.Header().Set("X-Test-UserID", gotUser.UserID)
		}
	}
	return w
}

// ---------- Extractor ----------

func TestExtractBearerToken(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"empty", "", "", false},
		{"only whitespace", "   ", "", false},
		{"no scheme", "sometoken", "", false},
		{"wrong scheme", "Basic abc", "", false},
		{"missing token", "Bearer ", "", false},
		{"canonical", "Bearer abc.def.ghi", "abc.def.ghi", true},
		{"lowercase scheme", "bearer abc.def.ghi", "abc.def.ghi", true},
		{"extra spaces", "  Bearer   abc.def.ghi  ", "abc.def.ghi", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractBearerToken(tc.in)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------- End-to-end middleware ----------

func TestRequireAuth_MissingHeader(t *testing.T) {
	m := newBearer(t, "dev-secret")
	w := runMiddleware(t, m, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NotEqual(t, "true", w.Header().Get("X-Test-NextCalled"))
}

func TestRequireAuth_SecretEmpty(t *testing.T) {
	m := newBearer(t, "")
	token := signJWT(t, "anything", map[string]any{"sub": "u-1", "exp": time.Unix(1_700_000_000, 0).Add(time.Hour).Unix()})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_WrongSignature(t *testing.T) {
	m := newBearer(t, "server-secret")
	token := signJWT(t, "other-secret", map[string]any{
		"sub": "u-1",
		"exp": time.Unix(1_700_000_000, 0).Add(time.Hour).Unix(),
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_Expired(t *testing.T) {
	m := newBearer(t, "dev-secret")
	token := signJWT(t, "dev-secret", map[string]any{
		"sub": "u-1",
		"exp": time.Unix(1_700_000_000, 0).Add(-time.Minute).Unix(), // past
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_MissingExp(t *testing.T) {
	m := newBearer(t, "dev-secret")
	// No exp claim — must be rejected, not silently accepted as "forever".
	token := signJWT(t, "dev-secret", map[string]any{
		"sub":   "u-1",
		"email": "admin@example.com",
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ExpZero(t *testing.T) {
	m := newBearer(t, "dev-secret")
	token := signJWT(t, "dev-secret", map[string]any{
		"sub": "u-1",
		"exp": 0,
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_MissingSub(t *testing.T) {
	m := newBearer(t, "dev-secret")
	token := signJWT(t, "dev-secret", map[string]any{
		"email": "admin@example.com",
		"role":  "owner",
		"exp":   time.Unix(1_700_000_000, 0).Add(time.Hour).Unix(),
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_Success(t *testing.T) {
	m := newBearer(t, "dev-secret")
	token := signJWT(t, "dev-secret", map[string]any{
		"sub":   "u-1",
		"email": "admin@example.com",
		"role":  "owner",
		"exp":   time.Unix(1_700_000_000, 0).Add(time.Hour).Unix(),
		"iat":   time.Unix(1_700_000_000, 0).Unix(),
	})
	w := runMiddleware(t, m, "Bearer "+token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Test-NextCalled"))
	assert.Equal(t, "u-1", w.Header().Get("X-Test-UserID"))
}

// ---------- Context helpers ----------

func TestGetAuthUser_Missing(t *testing.T) {
	assert.Nil(t, GetAuthUser(context.Background()))
}

func TestContextWithAuthUser_RoundTrip(t *testing.T) {
	u := &AuthUser{UserID: "u-1", Email: "x@y.com", Role: "admin"}
	ctx := ContextWithAuthUser(context.Background(), u)
	got := GetAuthUser(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "u-1", got.UserID)
	assert.Equal(t, "admin", got.Role)
}
