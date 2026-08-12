// Package ai is the vertical slice for the two read endpoints the internal AI
// dashboard uses:
//
//	GET /api/v1/ai/lessons   lessons of one tenant, with their full hierarchy
//	GET /api/v1/ai/tenants   tenants that have AI turned on
//
// Both are internal: they are guarded by the x-internal-api-key header rather
// than by a tenant API key, so they are not reachable by customers.
//
// The rest of `/api/v1/ai` — enqueueing transcription jobs, polling their
// status, patching a lesson's transcription flag — belongs to the
// transcription slice, which registers its own routes under the same prefix.
package ai

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db  *sql.DB
	log logger.Logger
	// internalAPIKey gates both endpoints. It comes from the validated config,
	// so it can never be empty here.
	internalAPIKey string
}

// New builds the slice.
func New(db *sql.DB, cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{db: db, log: log, internalAPIKey: cfg.Auth.InternalAPIKey}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	RateLimitTenant func(http.Handler) http.Handler
}

// authorized checks the internal API key. An empty incoming key is rejected
// outright so that a misconfigured deployment cannot leave the endpoint open.
func (f *Feature) authorized(w http.ResponseWriter, r *http.Request) bool {
	apiKey := r.Header.Get("x-internal-api-key")
	if apiKey == "" || apiKey != f.internalAPIKey {
		writeCustomError(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return false
	}
	return true
}
