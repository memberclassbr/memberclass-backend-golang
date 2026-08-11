package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/session"
	"golang.org/x/crypto/hkdf"
)

// AuthMiddleware authenticates a request by the NextAuth session cookie. The
// cookie is a JWE encrypted with a key derived from the shared NextAuth
// secret; this middleware derives the same key, decrypts, checks expiry, and
// confirms the user still exists.
type AuthMiddleware struct {
	db     *sql.DB
	log    logger.Logger
	secret []byte
}

// NewAuthMiddleware builds the middleware from the validated config.
//
// The secret is required. It used to fall back to a literal compiled into this
// file when NEXTAUTH_SECRET was unset, which meant a deployment that forgot the
// variable accepted session cookies forged by anyone who had read the source.
// Config now refuses to start without it.
func NewAuthMiddleware(db *sql.DB, cfg *config.Config, log logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{db: db, log: log, secret: []byte(cfg.Auth.NextAuthSecret)}
}

const sqlUserExists = `SELECT EXISTS(SELECT 1 FROM "User" WHERE id = $1)`

// Authenticate rejects the request unless the session cookie decrypts, is
// unexpired, and names a user that still exists.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("next-auth.session-token")
		if err != nil {
			m.log.Debug("Cookie next-auth.session-token not found")
			m.sendError(w, http.StatusUnauthorized, "Session token not found")
			return
		}

		payload, err := m.decryptToken(cookie.Value)
		if err != nil {
			m.log.Error("Failed to decrypt token: " + err.Error())
			m.sendError(w, http.StatusUnauthorized, "Invalid session token")
			return
		}

		if payload.Exp > 0 && time.Now().Unix() > payload.Exp {
			m.log.Debug("Token expired")
			m.sendError(w, http.StatusUnauthorized, "Session expired")
			return
		}

		// A valid cookie for a deleted account must not authenticate.
		exists, err := m.userExists(r.Context(), payload.User.ID)
		if err != nil {
			m.log.Error("Failed to check user existence: " + err.Error())
			m.sendError(w, http.StatusInternalServerError, "Failed to validate user")
			return
		}
		if !exists {
			m.log.Debug("User not found in database: " + payload.User.ID)
			m.sendError(w, http.StatusNotFound, "user not found")
			return
		}

		ctx := context.WithValue(r.Context(), session.ContextKey, payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) userExists(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var exists bool
	if err := m.db.QueryRowContext(ctx, sqlUserExists, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// deriveEncryptionKey reproduces NextAuth's key derivation: HKDF-SHA256 over
// the shared secret with a fixed info string and an empty salt. The info
// string is part of NextAuth's wire format and must match byte for byte.
func (m *AuthMiddleware) deriveEncryptionKey() ([]byte, error) {
	reader := hkdf.New(sha256.New, m.secret, nil, []byte("NextAuth.js Generated Encryption Key"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func (m *AuthMiddleware) decryptToken(tokenString string) (*session.Payload, error) {
	key, err := m.deriveEncryptionKey()
	if err != nil {
		return nil, err
	}

	jwe, err := jose.ParseEncrypted(tokenString,
		[]jose.KeyAlgorithm{jose.DIRECT},
		[]jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return nil, err
	}

	decrypted, err := jwe.Decrypt(key)
	if err != nil {
		return nil, err
	}

	var payload session.Payload
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (m *AuthMiddleware) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}
