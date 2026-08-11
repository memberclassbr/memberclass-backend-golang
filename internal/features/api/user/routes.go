package user

import "github.com/go-chi/chi/v5"

// Register mounts the routes under `/api/v1/user`.
//
// Note that `/informations` carries only the auth middleware. That is how the
// endpoint has always been mounted; adding the tenant rate limit here would
// change behaviour for existing callers.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal).Get("/informations", f.GetUserInformations)

	r.With(mw.AuthExternal, mw.RateLimitTenant).
		Get("/lessons/completed", f.GetLessonsCompleted)
}

// RegisterUsers mounts the routes under `/api/v1/users`, the plural prefix the
// purchases endpoint has always used.
func (f *Feature) RegisterUsers(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).
		Get("/purchases", f.GetUserPurchases)
}
