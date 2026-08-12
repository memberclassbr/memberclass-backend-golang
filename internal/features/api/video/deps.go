// Package video is the vertical slice for `POST /videos/upload`: it takes a
// multipart upload, resolves the tenant's Bunny library credentials, and
// pushes the file to Bunny Stream.
//
// The endpoint used to carry no credential at all — only the upload rate
// limiter — and lived under `/api/v1`. It now sits at the root with the other
// frontend-origin routes and requires a go-token Bearer JWT whose holder has
// any role in the tenantId named in the form.
package video

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/shared/tenantrole"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db    *sql.DB
	bunny bunny.Service
	log   logger.Logger
	roles *tenantrole.Checker
}

// New builds the slice.
func New(db *sql.DB, bunnySvc bunny.Service, log logger.Logger) *Feature {
	return &Feature{db: db, bunny: bunnySvc, log: log, roles: tenantrole.New(db)}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. The router owns middleware construction; slices just compose them.
type MiddlewareSet struct {
	// BearerAuth verifies the frontend's go-token JWT. The role check that
	// follows it is a helper the handler calls, because the tenant arrives as
	// a multipart field rather than in the URL.
	BearerAuth func(http.Handler) http.Handler
	// CheckUploadLimit rejects the request when the caller is over their byte
	// quota; IncrementAfterUpload charges the quota once the upload succeeds.
	// The quota is keyed on the go-token's `sub`, so both must sit below
	// BearerAuth.
	CheckUploadLimit     func(http.Handler) http.Handler
	IncrementAfterUpload func(http.Handler) http.Handler
}
