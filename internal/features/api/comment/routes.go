package comment

import "github.com/go-chi/chi/v5"

// Register mounts the routes under `/api/v1/comments`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).Group(func(r chi.Router) {
		r.Get("/", f.GetComments)
		r.Patch("/{commentID}", f.UpdateComment)
	})
}
