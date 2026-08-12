// Package auth is the vertical slice for `POST /api/v1/auth`, which mints a
// magic login link for a member of the authenticated tenant.
//
// The link carries a short code and nothing else — no token, no address. The
// frontend resolves the code to a "MagicToken" row and exchanges it for a
// session; see [magiclink] for the contract that shape has to satisfy.
//
// [magiclink]: github.com/memberclass-backend-golang/internal/shared/magiclink
package auth

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	cache cache.Cache
	log   logger.Logger
	// publicDomain is the frontend's root domain, used to build the tenant's
	// login URL when the tenant has no custom domain. It is the frontend's
	// host, not this service's: the member clicks the link in a browser.
	publicDomain string
}

// New builds the slice.
func New(db *sql.DB, c cache.Cache, cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{db: db, cache: c, log: log, publicDomain: cfg.Public.DomainURL}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
