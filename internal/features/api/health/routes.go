package health

import "github.com/go-chi/chi/v5"

// Register mounts the slice on r, which is expected to be the root router:
// the path is /health, not /api/v1/health, because that is where deployment
// platforms look by default.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Get("/health", f.Check)
}
