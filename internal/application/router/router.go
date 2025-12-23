package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/middlewares"
)

type Router struct {
	chi.Router
	videoHandler            *http.VideoHandler
	lessonHandler           *http.LessonHandler
	commentHandler          *http.CommentHandler
	userActivityHandler     *http.UserActivityHandler
	userPurchaseHandler     *http.UserPurchaseHandler
	userInformationsHandler *http.UserInformationsHandler
	socialCommentHandler    *http.SocialCommentHandler
	activitySummaryHandler  *http.ActivitySummaryHandler
	lessonsCompletedHandler  *http.LessonsCompletedHandler
	studentReportHandler     *http.StudentReportHandler
	rateLimitMiddleware     *middlewares.RateLimitMiddleware
	authMiddleware          *middlewares.AuthMiddleware
	authExternalMiddleware  *middlewares.AuthExternalMiddleware
}

func NewRouter(
	videoHandler *http.VideoHandler,
	lessonHandler *http.LessonHandler,
	commentHandler *http.CommentHandler,
	userActivityHandler *http.UserActivityHandler,
	userPurchaseHandler *http.UserPurchaseHandler,
	userInformationsHandler *http.UserInformationsHandler,
	socialCommentHandler *http.SocialCommentHandler,
	activitySummaryHandler *http.ActivitySummaryHandler,
	lessonsCompletedHandler *http.LessonsCompletedHandler,
	studentReportHandler *http.StudentReportHandler,
	rateLimitMiddleware *middlewares.RateLimitMiddleware,
	authMiddleware *middlewares.AuthMiddleware,
	authExternalMiddleware *middlewares.AuthExternalMiddleware,
) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	return &Router{
		Router:                  router,
		videoHandler:            videoHandler,
		lessonHandler:           lessonHandler,
		commentHandler:          commentHandler,
		userActivityHandler:     userActivityHandler,
		userPurchaseHandler:     userPurchaseHandler,
		userInformationsHandler: userInformationsHandler,
		socialCommentHandler:    socialCommentHandler,
		activitySummaryHandler:  activitySummaryHandler,
		lessonsCompletedHandler: lessonsCompletedHandler,
		studentReportHandler:    studentReportHandler,
		rateLimitMiddleware:     rateLimitMiddleware,
		authMiddleware:          authMiddleware,
		authExternalMiddleware:  authExternalMiddleware,
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

		router.Route("/comments", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/", r.commentHandler.GetComments)
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Patch("/{commentID}", r.commentHandler.UpdateComment)
		})

		router.Route("/user", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/informations", r.userInformationsHandler.GetUserInformations)
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/activities", r.userActivityHandler.GetUserActivities)
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/activity/summary", r.activitySummaryHandler.GetActivitySummary)
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/lessons/completed", r.lessonsCompletedHandler.GetLessonsCompleted)
		})

		router.Route("/users", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/purchases", r.userPurchaseHandler.GetUserPurchases)
		})

		router.Route("/social", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Post("/", r.socialCommentHandler.CreateOrUpdatePost)
		})

		router.Route("/student", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/report", r.studentReportHandler.GetStudentReport)
		})

	})

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
