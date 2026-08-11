// Package auth is the vertical slice for `POST /api/v1/auth`, which mints a
// magic login link for a member of the authenticated tenant.
//
// The link carries a one-time token; only its bcrypt hash is stored, and the
// frontend exchanges the token for a session.
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
	// rootDomain is this service's own host, used to build the tenant's login
	// URL when the tenant has no custom domain.
	rootDomain string
}

// New builds the slice.
func New(db *sql.DB, c cache.Cache, cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{db: db, cache: c, log: log, rootDomain: cfg.Public.RootDomain}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
