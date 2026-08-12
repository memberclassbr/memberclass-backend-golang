// Package vitrine is the vertical slice for the tenant-facing catalog under
// `/api/v1/vitrine`: the storefront listing plus lookups for a single vitrine,
// course, module or lesson.
//
// The catalog is a five-level tree — vitrine → course → section → module →
// lesson. Every endpoint that returns a subtree loads it with one query per
// level and groups the rows in memory, rather than issuing a query per parent.
package vitrine

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db  *sql.DB
	log logger.Logger
}

// New builds the slice.
func New(db *sql.DB, log logger.Logger) *Feature {
	return &Feature{db: db, log: log}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
