// Package comment is the vertical slice for lesson comments:
//
//	GET   /api/v1/comments              list, filtered and paginated
//	PATCH /api/v1/comments/{commentID}  answer and publish
//
// The same listing was also mounted at `GET /api/comments` behind the NextAuth
// session cookie. That route is gone: it had no callers, and it was the only
// thing on this service reachable with a credential a browser attaches by
// itself — which is what forced the CORS policy to allow credentials.
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
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
