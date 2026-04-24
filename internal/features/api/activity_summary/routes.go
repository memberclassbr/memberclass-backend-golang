package activity_summary

import "github.com/go-chi/chi/v5"

// Register mounts the slice's HTTP routes on r. r is expected to already be
// scoped to `/api/v1/user`; paths below are written relative to that prefix.
// (The scope will dissolve once every slice under /user migrates.)
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).
		Get("/activity/summary", f.GetActivitySummary)
}
