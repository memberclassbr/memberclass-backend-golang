// Package video is the vertical slice for `POST /api/v1/videos/upload`: it
// takes a multipart upload, resolves the tenant's Bunny library credentials,
// and pushes the file to Bunny Stream.
//
// Note that this endpoint carries no credential check of its own — only the
// upload rate limiter. That is how it has always been mounted.
package video

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	bunny bunny.Service
	log   logger.Logger
}

// New builds the slice.
func New(db *sql.DB, bunnySvc bunny.Service, log logger.Logger) *Feature {
	return &Feature{db: db, bunny: bunnySvc, log: log}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	// CheckUploadLimit rejects the request when the tenant is over its byte
	// quota; IncrementAfterUpload charges the quota once the upload succeeds.
	CheckUploadLimit     func(http.Handler) http.Handler
	IncrementAfterUpload func(http.Handler) http.Handler
}
