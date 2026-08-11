package auth

import "github.com/go-chi/chi/v5"

// Register mounts the slice's route on r, which is expected to be scoped to
// `/api/v1/auth`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).
		Post("/", f.GenerateMagicLink)
}
