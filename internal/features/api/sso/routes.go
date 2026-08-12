package sso

import "github.com/go-chi/chi/v5"

// Register mounts generate-token on r, which is expected to be scoped to
// `/sso` — the root prefix the admin frontend calls with a Bearer JWT.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.BearerAuth, mw.RateLimitTenant).Post("/generate-token", f.GenerateSSOToken)
}

// RegisterTenantAPI mounts validate-token on r, which is expected to be scoped
// to `/api/v1/sso`. This is the half a tenant's own site calls, so it keeps the
// external API key it has always used: that site holds no NextAuth session and
// has no way to mint the Bearer the other half now requires.
func (f *Feature) RegisterTenantAPI(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal).Post("/validate-token", f.ValidateSSOToken)
}
