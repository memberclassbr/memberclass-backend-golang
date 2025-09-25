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
	rateLimitMiddleware *middlewares.RateLimitMiddleware
}

func NewRouter(videoHandler *http.VideoHandler, rateLimiter ports.RateLimiterUpload, logger ports.Logger) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	rateLimitMiddleware := middlewares.NewRateLimitMiddleware(rateLimiter, logger)

	return &Router{
		Router:              router,
		videoHandler:        videoHandler,
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
}
