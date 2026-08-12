package video

import "github.com/go-chi/chi/v5"

// Register mounts the slice's route on r, which is expected to be scoped to
// `/videos`.
//
// BearerAuth goes first for two reasons: an unauthenticated caller is rejected
// before the upload quota is consulted, and the quota is keyed on the token's
// `sub` — CheckUploadLimit reads the identity off the context the Bearer
// middleware populates, so mounted the other way round it has nothing to key on
// and answers 401.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.BearerAuth, mw.CheckUploadLimit, mw.IncrementAfterUpload).
		Post("/upload", f.UploadVideo)
}
