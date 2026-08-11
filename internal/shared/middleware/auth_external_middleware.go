// Package middleware holds the HTTP middlewares shared across slices: the
// three credential checks and the three rate limiters. They live here rather
// than inside a slice because every slice composes them, and none owns them.
package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// sqlTenantByToken resolves a hashed API key to its tenant.
const sqlTenantByToken = `
	SELECT id, name
	FROM "Tenant"
	WHERE token_api_auth = $1
	LIMIT 1
`

// AuthExternalMiddleware authenticates a tenant by the `mc-api-key` header.
// The header carries the plaintext key; the database stores its SHA-256, so
// the middleware hashes before looking up.
type AuthExternalMiddleware struct {
	db  *sql.DB
	log logger.Logger
}

func NewAuthExternalMiddleware(db *sql.DB, log logger.Logger) *AuthExternalMiddleware {
	return &AuthExternalMiddleware{db: db, log: log}
}

// Authenticate rejects the request unless the key resolves to a tenant, and
// puts that tenant in the request context for downstream handlers.
func (m *AuthExternalMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("mc-api-key")
		if key == "" {
			sendInvalidAPIKey(w)
			return
		}

		found, err := m.tenantByKey(r.Context(), key)
		if err != nil {
			// A lookup failure and an unknown key are reported identically:
			// the response must not tell a caller whether a key exists.
			sendInvalidAPIKey(w)
			return
		}

		ctx := context.WithValue(r.Context(), tenant.ContextKey, found)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthExternalMiddleware) tenantByKey(ctx context.Context, key string) (*tenant.Tenant, error) {
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	var found tenant.Tenant
	err := m.db.QueryRowContext(ctx, sqlTenantByToken, hash).Scan(&found.ID, &found.Name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, err
	case err != nil:
		m.log.Error("Error finding tenant with token: " + err.Error())
		return nil, err
	}
	return &found, nil
}

func sendInvalidAPIKey(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        false,
		"error":     "API key invalid",
		"errorCode": "INVALID_API_KEY",
	})
}
