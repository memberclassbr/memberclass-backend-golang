package sso

import "github.com/go-chi/chi/v5"

// Register mounts the slice's routes on r, which is expected to be scoped to
// `/api/v1/sso`.
//
// The two endpoints are guarded differently, as they always have been:
// generate-token checks the internal API key inside the handler and carries
// the tenant rate limit; validate-token is called by the tenant's own site and
// goes through the external API key middleware.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.RateLimitTenant).Post("/generate-token", f.GenerateSSOToken)
	r.With(mw.AuthExternal).Post("/validate-token", f.ValidateSSOToken)
}
