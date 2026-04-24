// Bearer JWT auth for frontend-origin routes.
//
// The Next.js frontend mints a short-lived HS256 JWT at `/api/auth/go-token`
// using NEXTAUTH_SECRET and sends it on `Authorization: Bearer <jwt>`. This
// middleware verifies the signature + expiry with the same secret and makes
// the caller's identity available to downstream handlers via GetAuthUser.
//
// Expected JWT claims:
//
//	{
//	  "sub":   "<userId>",
//	  "email": "<email>",
//	  "role":  "<session role>",
//	  "exp":   <unix>,
//	  "iat":   <unix>
//	}
//
// No tenant is carried on the token — per-tenant authorization is the
// responsibility of each slice (query `UsersOnTenants` for the target
// tenantId and enforce the required role).
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// AuthUser is the payload every RequireAuth-protected handler gets from the
// request context via GetAuthUser.
type AuthUser struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Exp    int64  `json:"exp"`
	Iat    int64  `json:"iat"`
}

// authUserKey is the unexported context key under which *AuthUser is stored.
// Using a typed-struct key (not a string) avoids accidental collisions with
// other middlewares.
type authUserKey struct{}

// GetAuthUser returns the authenticated user attached to ctx by RequireAuth,
// or nil if the request never passed through the middleware.
func GetAuthUser(ctx context.Context) *AuthUser {
	u, _ := ctx.Value(authUserKey{}).(*AuthUser)
	return u
}

// ContextWithAuthUser attaches `user` to ctx under the same key RequireAuth
// uses. Intended for tests (and future internal callers that want to reuse
// the AuthUser shape) — never call this from a handler to impersonate a
// user, since it bypasses the JWT verification flow.
func ContextWithAuthUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, authUserKey{}, user)
}

// BearerMiddleware verifies JWT HS256 Authorization headers.
type BearerMiddleware struct {
	logger ports.Logger
	secret []byte
	now    func() time.Time // overridable for tests
}

// NewBearerMiddleware builds the middleware. Reads NEXTAUTH_SECRET from env;
// MUST match the Next.js frontend's secret byte-for-byte.
func NewBearerMiddleware(logger ports.Logger) *BearerMiddleware {
	secret := os.Getenv("NEXTAUTH_SECRET")
	if secret == "" {
		// Keep the app bootable so routes that don't require bearer auth
		// still serve; every request to RequireAuth will simply reject.
		logger.Warn("NEXTAUTH_SECRET is empty — bearer auth will reject all requests")
	}
	return &BearerMiddleware{
		logger: logger,
		secret: []byte(secret),
		now:    time.Now,
	}
}

// RequireAuth is the chi-compatible middleware constructor. Wrap any route
// that must be called from the frontend with a valid go-token JWT:
//
//	r.With(bearer.RequireAuth).Post("/imports/members", h.Import)
func (m *BearerMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := extractBearerToken(r.Header.Get("Authorization"))
		if !ok {
			m.reject(w, "missing or malformed Authorization header")
			return
		}

		user, err := m.verify(raw)
		if err != nil {
			m.logger.Debug("bearer auth rejected: " + err.Error())
			m.reject(w, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), authUserKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// verify parses a compact JWT, checks the HS256 signature with the shared
// secret, then validates the `exp` claim. Returns the decoded AuthUser on
// success.
//
// Rolled by hand rather than via go-jose so secrets shorter than 32 bytes
// are accepted (Node's `jsonwebtoken` — the library the Next.js frontend
// uses to mint this token — has no length check; enforcing one here would
// reject every real NEXTAUTH_SECRET in production today).
func (m *BearerMiddleware) verify(raw string) (*AuthUser, error) {
	if len(m.secret) == 0 {
		return nil, errors.New("server is not configured with NEXTAUTH_SECRET")
	}

	payload, err := verifyHS256(raw, m.secret)
	if err != nil {
		return nil, err
	}

	var user AuthUser
	if err := json.Unmarshal(payload, &user); err != nil {
		return nil, err
	}

	if user.UserID == "" {
		return nil, errors.New("token payload missing sub claim")
	}
	// `exp` is REQUIRED. A token without exp (or exp==0) would be valid
	// forever — combined with a leaked NEXTAUTH_SECRET that's permanent
	// admin access. Reject rather than let the lifetime float.
	if user.Exp == 0 {
		return nil, errors.New("token payload missing exp claim")
	}
	if m.now().Unix() >= user.Exp {
		return nil, errors.New("token expired")
	}

	return &user, nil
}

// verifyHS256 parses a compact JWT ("header.payload.signature"), checks
// alg=HS256 on the header, and verifies the HMAC-SHA256 signature over
// "header.payload" against `secret`. Returns the raw JSON payload bytes
// on success — claim decoding is the caller's job.
func verifyHS256(token string, secret []byte) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid header encoding")
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("invalid header json")
	}
	if header.Alg != "HS256" {
		return nil, errors.New("unsupported algorithm: " + header.Alg)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)

	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}
	if !hmac.Equal(expected, actual) {
		return nil, errors.New("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}
	return payload, nil
}

// reject writes a uniform 401 JSON body so the frontend can detect auth
// failures and re-fetch the token (goFetch invalidates its cache on 401).
func (m *BearerMiddleware) reject(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   http.StatusText(http.StatusUnauthorized),
		"message": message,
	})
}

// extractBearerToken pulls `Bearer <token>` from the Authorization header.
// Whitespace-tolerant, case-insensitive on the scheme.
func extractBearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(header) <= len(prefix) {
		return "", false
	}
	if !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
