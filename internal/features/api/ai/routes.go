package ai

import "github.com/go-chi/chi/v5"

// Register mounts the slice's routes on r, which is expected to be scoped to
// `/api/v1/ai`.
//
// The lessons listing keeps the tenant rate limit it has always carried; the
// tenants listing has none.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.Route("/lessons", func(r chi.Router) {
		r.With(mw.RateLimitTenant).Get("/", f.GetLessons)
	})

	r.Route("/tenants", func(r chi.Router) {
		r.Get("/", f.GetTenantsWithAIEnabled)
	})
}
