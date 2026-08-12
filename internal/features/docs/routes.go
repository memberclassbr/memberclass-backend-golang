package docs

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Register mounts the slice's HTTP routes on r. r is expected to be the root
// router — the paths below are absolute.
//
// There is no MiddlewareSet parameter: the docs are public by design (no API
// key, no tenant scope, no rate limiter), which is the same exposure the old
// handler had.
func (f *Feature) Register(r chi.Router) {
	r.Get("/docs", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/docs/", http.StatusMovedPermanently)
	})
	r.Route("/docs", func(router chi.Router) {
		router.Get("/", f.ServeSwaggerUI)
		router.Get("/swagger.yaml", f.ServeSwaggerYAML)
	})
}
