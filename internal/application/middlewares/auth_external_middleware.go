package middlewares

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type AuthExternalMiddleware struct {
	ApiTokenUseCase ports.ApiTokenUseCase
}

func NewAuthExternalMiddleware(apiTokenUseCase ports.ApiTokenUseCase) *AuthExternalMiddleware {
	return &AuthExternalMiddleware{
		ApiTokenUseCase: apiTokenUseCase,
	}
}

func (aem *AuthExternalMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("mc-api-key")
		if key == "" {
			sendError(w)
			return
		}

		hash := aem.generateSHA256Hash(key)

		if hash == "" {
			sendError(w)
			return
		}

		tenant, err := aem.ApiTokenUseCase.ValidateToken(r.Context(), hash)
		if err != nil {
			sendError(w)
			return
		}

		ctx := context.WithValue(r.Context(), constants.TenantContextKey, tenant)
		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

func (aem *AuthExternalMiddleware) generateSHA256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

func sendError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        false,
		"error":     "API key invalid",
		"errorCode": "INVALID_API_KEY",
	})
	if err != nil {
		return
	}
}
