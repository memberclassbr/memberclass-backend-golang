// Package user_activities is a vertical slice for the
// `GET /api/v1/user/activities` endpoint. Returns a unified timeline of a
// single user's events within a tenant: logins, completed lessons, downloads,
// comments, terms acceptances, quizzes and certificates.
//
// See CLAUDE.md ("Architecture migration in progress") for the target
// structure and rules for new features during the migration.
package user_activities

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	cache ports.Cache
	log   ports.Logger
}

// New builds the slice. Wire it in cmd/api/main.go via fx.Provide.
func New(db *sql.DB, cache ports.Cache, log ports.Logger) *Feature {
	return &Feature{db: db, cache: cache, log: log}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	AuthExternal    func(http.Handler) http.Handler
	RateLimitTenant func(http.Handler) http.Handler
}
