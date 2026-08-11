package video

import "github.com/go-chi/chi/v5"

// Register mounts the slice's route on r, which is expected to be scoped to
// `/api/v1/videos`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.CheckUploadLimit, mw.IncrementAfterUpload).
		Post("/upload", f.UploadVideo)
}
