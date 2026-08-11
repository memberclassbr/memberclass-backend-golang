// Package student is the vertical slice for `GET /api/v1/student/report`, the
// tenant-facing roster export: every student assigned to the tenant, with the
// deliveries they are linked to, the lessons they have watched and their last
// login.
package student

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
