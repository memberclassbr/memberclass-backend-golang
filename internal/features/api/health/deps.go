// Package health answers the platform's question: should this container keep
// receiving traffic?
//
// The check reaches the two dependencies without which no endpoint works — the
// tenant database and Redis — because a process that is running but cannot
// reach its database is exactly the case a liveness probe on the process alone
// misses.
//
// The response body says whether the service is healthy and nothing else. This
// route carries no credential, so naming which dependency failed, or repeating
// the driver's error, would hand an unauthenticated caller a map of the
// deployment's internals. The status code is all the platform reads.
package health

import (
	"database/sql"

	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature is the slice's dependencies.
type Feature struct {
	db    *sql.DB
	cache cache.Cache
	log   logger.Logger
}

// MiddlewareSet is empty: the route is deliberately unguarded so the platform
// can probe it without holding a credential.
type MiddlewareSet struct{}

func New(db *sql.DB, c cache.Cache, log logger.Logger) *Feature {
	return &Feature{db: db, cache: c, log: log}
}
