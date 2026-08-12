// Bearer JWT auth for frontend-origin routes.
//
// The Next.js frontend mints a short-lived HS256 JWT at
// `/api/auth/go-token?tenantId=X` and sends it on `Authorization: Bearer <jwt>`.
// Before minting, it checks that the session's user holds a row in
// "UsersOnTenants" for X and refuses with 403 if not. This middleware verifies
// the signature, audience and expiry, then makes the caller's identity
// available to downstream handlers via GetAuthUser.
//
// Expected JWT claims:
//
//	{
//	  "sub":      "<userId>",
//	  "email":    "<email>",
//	  "tenantId": "<the tenant this token is scoped to>",
//	  "role":     "<the caller's role IN THAT TENANT, read from the database>",
//	  "aud":      "memberclass-go-api",
//	  "jti":      "<opaque id, optional, for revocation>",
//	  "exp":      <unix>,
//	  "iat":      <unix>
//	}
//
// The token is scoped to one tenant. `tenantId` is required and it is the only
// tenant a request carrying this token may act on — a handler that read the
// tenant out of a request body instead would give the scope away, since the
// body is chosen by the caller and the claim is not.
//
// `role` is now trustworthy in a way it was not before: it is read from
// "UsersOnTenants" at mint time for the tenant the token names. It is still a
// snapshot up to the token's lifetime old, so authorization re-reads the row —
// see internal/shared/tenantrole.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Audience is the `aud` claim this service accepts, and the only one.
//
// Without it, any service that verifies HS256 against the same secret would
// take a go-token as its own. It is cheap to check and it is what stops one
// token from being replayed at a second audience.
const Audience = "memberclass-go-api"

// AuthUser is the payload every RequireAuth-protected handler gets from the
// request context via GetAuthUser.
type AuthUser struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	// TenantID is the tenant this token was minted for, and the only one the
	// request may act on.
	TenantID string `json:"tenantId"`
	// Role is the caller's role in TenantID as the frontend read it at mint
	// time. Treat it as a hint; tenantrole re-reads the row.
	Role     string   `json:"role"`
	Audience audience `json:"aud"`
	// ID is the `jti` claim. Optional — see BearerMiddleware.revoked.
	ID  string `json:"jti"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

// audience decodes the `aud` claim, which RFC 7519 allows to be either a single
// string or an array of them. Node's `jsonwebtoken` emits a bare string for one
// audience and an array for several, so accept both rather than depend on which
// shape the frontend happens to produce.
type audience []string

func (a *audience) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*a = audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return errors.New("aud claim is neither a string nor an array of strings")
	}
	*a = many
	return nil
}

func (a audience) contains(want string) bool { return slices.Contains(a, want) }

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
	logger logger.Logger
	secret []byte
	cache  cache.Cache
	now    func() time.Time // overridable for tests
}

// NewBearerMiddleware builds the middleware from the validated config. The
// secret MUST match the Next.js frontend's byte-for-byte; config refuses to
// start without it and enforces a length floor, so it is never empty or short
// here.
//
// `c` backs the revocation denylist and may be nil, which disables that check.
func NewBearerMiddleware(cfg *config.Config, c cache.Cache, log logger.Logger) *BearerMiddleware {
	return &BearerMiddleware{
		logger: log,
		secret: []byte(cfg.Auth.GoAPIJWTSecret),
		cache:  c,
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

		if m.revoked(r.Context(), user) {
			m.logger.Debug("bearer auth rejected: token is on the revocation denylist")
			m.reject(w, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), authUserKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RevocationKey is where the frontend publishes a `jti` it wants dead before
// the token's own exp — a logout, or an access revoked mid-window. Give the key
// a TTL at least as long as the token's remaining lifetime; anything longer is
// only wasted memory.
func RevocationKey(jti string) string { return "go-token:revoked:" + jti }

// revoked reports whether this token's jti has been published to the denylist.
//
// A token with no `jti` cannot be revoked and is not rejected for it: the claim
// is the frontend's to emit, and refusing tokens that lack it would turn a
// defence-in-depth check into an outage the first time the two sides deploy out
// of step.
//
// A Redis failure fails OPEN, logged. What the denylist shortens is a window
// already bounded by `exp` — minutes — while failing closed would take every
// admin route down whenever Redis blinks. Role changes do not depend on this
// path at all: tenantrole re-reads "UsersOnTenants" on every request, so a
// demotion lands immediately whatever Redis is doing.
func (m *BearerMiddleware) revoked(ctx context.Context, user *AuthUser) bool {
	if m.cache == nil || user.ID == "" {
		return false
	}
	found, err := m.cache.Exists(ctx, RevocationKey(user.ID))
	if err != nil {
		m.logger.Error("bearer auth: revocation lookup failed, allowing the request", "error", err.Error())
		return false
	}
	return found
}

// verify parses a compact JWT, checks the HS256 signature with the shared
// secret, then validates the claims this service requires. Returns the decoded
// AuthUser on success.
//
// Every check below rejects rather than defaults. A claim that is merely
// absent is the shape a forged or a stale token takes, and defaulting any of
// them — no audience, no tenant, no expiry — widens what the token grants.
func (m *BearerMiddleware) verify(raw string) (*AuthUser, error) {
	if len(m.secret) == 0 {
		return nil, errors.New("server is not configured with GO_API_JWT_SECRET")
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
	// Without `aud` any service verifying HS256 against this secret would
	// accept a go-token as its own.
	if !user.Audience.contains(Audience) {
		return nil, errors.New("token audience is not " + Audience)
	}
	// `tenantId` is the token's whole scope. A token without one would have to
	// fall back on a tenant named somewhere the caller controls, which is the
	// scoping bug this claim exists to close.
	if user.TenantID == "" {
		return nil, errors.New("token payload missing tenantId claim")
	}
	// `exp` is REQUIRED. A token without exp (or exp==0) would be valid
	// forever — combined with a leaked secret that is permanent admin access.
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
