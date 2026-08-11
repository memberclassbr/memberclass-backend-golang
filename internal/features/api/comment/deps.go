// Package comment is the vertical slice for lesson comments:
//
//	GET   /api/v1/comments              list, filtered and paginated
//	PATCH /api/v1/comments/{commentID}  answer and publish
//	GET   /api/comments                 the same listing behind the legacy
//	                                    mc-api-key middleware
//
// Both listing routes run the same handler; only the middleware in front of
// them differs.
package comment

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
	// AuthExternal validates the tenant-facing external API key.
	AuthExternal func(http.Handler) http.Handler
	// AuthAPIKey validates the mc-api-key header used by the legacy route.
	AuthAPIKey      func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
