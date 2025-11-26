package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/memberclass-backend-golang/internal/application/handlers/http"

	"github.com/memberclass-backend-golang/internal/application/middlewares"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type Router struct {
	chi.Router
	videoHandler        *http.VideoHandler
	lessonHandler       *http.LessonHandler
	rateLimitMiddleware *middlewares.RateLimitMiddleware
}

func NewRouter(videoHandler *http.VideoHandler, lessonHandler *http.LessonHandler, rateLimiter ports.RateLimiterUpload, logger ports.Logger) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	rateLimitMiddleware := middlewares.NewRateLimitMiddleware(rateLimiter, logger)

	return &Router{
		Router:              router,
		videoHandler:        videoHandler,
		lessonHandler:       lessonHandler,
		rateLimitMiddleware: rateLimitMiddleware,
	}
}

func (r *Router) SetupRoutes() {
	r.Route("/api/v1", func(router chi.Router) {

		router.Route("/videos", func(router chi.Router) {
			router.With(
				r.rateLimitMiddleware.CheckUploadLimit,
				r.rateLimitMiddleware.IncrementAfterUpload,
			).Post("/upload", r.videoHandler.UploadVideo)
		})

	})

	//routes for frontend nextJS
	r.Route("/api", func(router chi.Router) {
		router.Route("/lessons", func(router chi.Router) {
			router.Post("/pdf-process", r.lessonHandler.ProcessLesson)
			router.Post("/process-all-pdfs", r.lessonHandler.ProcessAllPendingLessons)
			router.Route("/{lessonId}", func(router chi.Router) {
				router.Post("/pdf-regenerate", r.lessonHandler.RegeneratePDF)
				router.Get("/pdf-pages", r.lessonHandler.GetLessonsPage)
			})
		})
	})
}
