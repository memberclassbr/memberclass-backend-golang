package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/memberclass-backend-golang/internal/application/handlers/http"
)

type Router struct {
	chi.Router
	videoHandler *http.VideoHandler
}

func NewRouter(videoHandler *http.VideoHandler) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	
	return &Router{
		Router:       router,
		videoHandler: videoHandler,
	}
}

func (r *Router) SetupRoutes() {
	r.Route("/api/v1", func(router chi.Router) {
		
		router.Route("/videos", func(router chi.Router) {
			router.Post("/upload", r.videoHandler.UploadVideo)
		})
	})
}