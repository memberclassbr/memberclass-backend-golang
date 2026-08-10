// Package user is the vertical slice for the tenant-facing endpoints about a
// member:
//
//	GET /api/v1/user/informations       roster with deliveries and last access
//	GET /api/v1/user/lessons/completed  lessons a member finished in a window
//	GET /api/v1/users/purchases         a member's purchase and refund events
//
// The two URL prefixes are historical — /user and /users — and are part of the
// contract, so the slice registers under both.
package user

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	cache cache.Cache
	log   logger.Logger
}

// New builds the slice.
func New(db *sql.DB, c cache.Cache, log logger.Logger) *Feature {
	return &Feature{db: db, cache: c, log: log}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
