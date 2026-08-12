package vitrine

import "github.com/go-chi/chi/v5"

// Register mounts the slice's routes on r, which is expected to be scoped to
// `/api/v1/vitrine`.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.AuthExternal, mw.RateLimitTenant).Group(func(r chi.Router) {
		r.Get("/", f.GetVitrines)
		r.Get("/{vitrineId}", f.GetVitrine)
		r.Get("/courses/{courseId}", f.GetCourse)
		r.Get("/modules/{moduleId}", f.GetModule)
		r.Get("/lessons/{lessonId}", f.GetLesson)
	})
}
