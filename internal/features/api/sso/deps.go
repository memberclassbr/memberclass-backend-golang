// Package sso is the vertical slice for single sign-on hand-off between the
// platform and a tenant's external site:
//
//	POST /sso/generate-token          mint a one-time token + redirect URL
//	POST /api/v1/sso/validate-token   consume it and return the identity
//
// The two live at different roots because they are called by different
// parties. generate-token is called by the platform's own frontend, so it
// moved to the root with the other frontend-origin routes and is guarded by a
// go-token Bearer JWT plus a role check. validate-token is called by the
// tenant's external site, which has no NextAuth session to mint a Bearer from,
// so it stays under `/api/v1` behind the tenant API key.
//
// The token is single-use and short-lived. Only its SHA-256 is stored, and
// consumption happens inside a transaction with SELECT ... FOR UPDATE so two
// concurrent redemptions cannot both succeed.
package sso

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/tenantrole"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	log   logger.Logger
	roles *tenantrole.Checker
}

// New builds the slice.
func New(db *sql.DB, log logger.Logger) *Feature {
	return &Feature{db: db, log: log, roles: tenantrole.New(db)}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	BearerAuth      func(http.Handler) http.Handler
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
