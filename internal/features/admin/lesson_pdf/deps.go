// Package lesson_pdf is the vertical slice for turning a lesson's PDF into
// page images, under the legacy `/api/lessons` prefix:
//
//	POST /api/lessons/pdf-process                 process one lesson, or run a
//	                                              maintenance action
//	POST /api/lessons/process-all-pdfs            process every pending lesson
//	POST /api/lessons/{lessonId}/pdf-regenerate   redo one lesson from scratch
//	GET  /api/lessons/{lessonId}/pdf-pages        read the rendered pages
//
// These are internal endpoints, guarded by the x-internal-api-key header
// rather than by a tenant key — hence admin/ rather than api/.
//
// The pipeline is: fetch the source PDF, hand it to iLovePDF for rasterising,
// upload each page image to this deployment's Spaces bucket, and record the
// page rows. Progress is tracked on a LessonPDFAsset row whose status moves
// pending → processing → done | partial | failed.
package lesson_pdf

import (
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/ilovepdf"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/storage"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	db      *sql.DB
	pdf     ilovepdf.Service
	storage storage.Storage
	log     logger.Logger
	// internalAPIKey gates every endpoint in this slice.
	internalAPIKey string
}

// New builds the slice. pdf may be nil when iLovePDF is not configured; the
// endpoints then fail at request time rather than blocking startup.
func New(db *sql.DB, pdf ilovepdf.Service, store storage.Storage, cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{
		db:             db,
		pdf:            pdf,
		storage:        store,
		log:            log,
		internalAPIKey: cfg.Auth.InternalAPIKey,
	}
}

// MiddlewareSet carries the chi-compatible middlewares the slice's routes
// need. These routes carry none today; the field exists so adding one does not
// change the Register signature.
type MiddlewareSet struct{}

// authorized validates the internal API key. An empty incoming key is rejected
// outright: without that guard an unset key would make a header-less request
// compare equal and pass.
func (f *Feature) authorized(w http.ResponseWriter, r *http.Request) bool {
	apiKey := r.Header.Get("x-internal-api-key")
	if apiKey == "" || apiKey != f.internalAPIKey {
		writeError(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório")
		return false
	}
	return true
}
