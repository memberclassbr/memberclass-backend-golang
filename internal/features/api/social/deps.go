// Package social is the vertical slice for `POST /api/v1/social`, the endpoint
// the community feed uses to publish or edit a post inside a topic.
//
// One endpoint covers both operations: a request carrying postId edits, one
// carrying topicId creates. The two paths differ mostly in their authorisation
// rules, which is the bulk of this slice.
package social

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
