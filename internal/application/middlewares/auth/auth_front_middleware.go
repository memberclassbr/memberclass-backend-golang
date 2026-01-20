package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/user"
	"golang.org/x/crypto/hkdf"
)

type AuthMiddleware struct {
	logger           ports.Logger
	sessionValidator user.SessionValidatorUseCase
	secret           []byte
}

func NewAuthMiddleware(logger ports.Logger, sessionValidator user.SessionValidatorUseCase) *AuthMiddleware {
	secret := os.Getenv("NEXTAUTH_SECRET")
	if secret == "" {
		secret = "WLcglAhaob5u0K8heuOSK7rONH7bdF"
	}
	return &AuthMiddleware{
		logger:           logger,
		sessionValidator: sessionValidator,
		secret:           []byte(secret),
	}
}

func (m *AuthMiddleware) deriveEncryptionKey() ([]byte, error) {
	hash := sha256.New
	info := []byte("NextAuth.js Generated Encryption Key")
	salt := make([]byte, 0)

	reader := hkdf.New(hash, m.secret, salt, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func (m *AuthMiddleware) decryptToken(tokenString string) (*dto.SessionPayload, error) {
	encryptionKey, err := m.deriveEncryptionKey()
	if err != nil {
		return nil, err
	}

	jwe, err := jose.ParseEncrypted(tokenString, []jose.KeyAlgorithm{jose.DIRECT}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return nil, err
	}

	decrypted, err := jwe.Decrypt(encryptionKey)
	if err != nil {
		return nil, err
	}

	var payload dto.SessionPayload
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

func (m *AuthMiddleware) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

func (m *AuthMiddleware) handleValidationError(w http.ResponseWriter, err error) {
	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		m.sendError(w, memberClassErr.Code, memberClassErr.Message)
		return
	}
	m.sendError(w, http.StatusInternalServerError, "Failed to validate user")
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("next-auth.session-token")
		if err != nil {
			m.logger.Debug("Cookie next-auth.session-token not found")
			m.sendError(w, http.StatusUnauthorized, "Session token not found")
			return
		}

		payload, err := m.decryptToken(cookie.Value)
		if err != nil {
			m.logger.Error("Failed to decrypt token: " + err.Error())
			m.sendError(w, http.StatusUnauthorized, "Invalid session token")
			return
		}

		if payload.Exp > 0 && time.Now().Unix() > payload.Exp {
			m.logger.Debug("Token expired")
			m.sendError(w, http.StatusUnauthorized, "Session expired")
			return
		}

		if err := m.sessionValidator.ValidateUserExists(payload.User.ID); err != nil {
			m.handleValidationError(w, err)
			return
		}

		ctx := context.WithValue(r.Context(), dto.UserContextKey, payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
