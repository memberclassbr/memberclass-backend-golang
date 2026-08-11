// Package sso is the vertical slice for single sign-on hand-off between the
// platform and a tenant's external site:
//
//	POST /api/v1/sso/generate-token   mint a one-time token + redirect URL
//	POST /api/v1/sso/validate-token   consume it and return the identity
//
// The token is single-use and short-lived. Only its SHA-256 is stored, and
// consumption happens inside a transaction with SELECT ... FOR UPDATE so two
// concurrent redemptions cannot both succeed.
package sso

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
	// internalAPIKey gates generate-token, which is called by the platform's
	// own frontend rather than by a customer.
	internalAPIKey string
}

// New builds the slice.
func New(db *sql.DB, cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{db: db, log: log, internalAPIKey: cfg.Auth.InternalAPIKey}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
