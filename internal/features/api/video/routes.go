package video

import "github.com/go-chi/chi/v5"

// Register mounts the slice's route on r, which is expected to be scoped to
// `/videos`.
//
// BearerAuth goes first so an unauthenticated caller is rejected before the
// upload quota is consulted — otherwise a stranger's request would still be
// charged against the tenant's bytes.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.BearerAuth, mw.CheckUploadLimit, mw.IncrementAfterUpload).
		Post("/upload", f.UploadVideo)
}
