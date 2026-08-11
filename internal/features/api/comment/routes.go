package comment

import "github.com/go-chi/chi/v5"

// Register mounts the routes under `/api/v1/comments`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).Group(func(r chi.Router) {
		r.Get("/", f.GetComments)
		r.Patch("/{commentID}", f.UpdateComment)
	})
}

// RegisterLegacy mounts the listing under `/api/comments`, the older prefix
// guarded by the mc-api-key middleware. Same handler, different credential.
func (f *Feature) RegisterLegacy(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthAPIKey).Get("/", f.GetComments)
}
