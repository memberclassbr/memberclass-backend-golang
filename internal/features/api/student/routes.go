package student

import "github.com/go-chi/chi/v5"

// Register mounts the slice's routes on r, which is expected to be scoped to
// `/api/v1/student`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).
		Get("/report", f.GetStudentReport)
}
