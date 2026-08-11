package lesson_pdf

import "github.com/go-chi/chi/v5"

// Register mounts the slice's routes on r, which is expected to be scoped to
// `/api/lessons`.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Post("/pdf-process", f.ProcessLesson)
	r.Post("/process-all-pdfs", f.ProcessAllPendingLessons)

	r.Route("/{lessonId}", func(r chi.Router) {
		r.Post("/pdf-regenerate", f.RegeneratePDF)
		r.Get("/pdf-pages", f.GetLessonsPage)
	})
}
