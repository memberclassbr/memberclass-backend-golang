package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	internalhttp "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/ai"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/auth"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/comment"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/lesson"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/sso"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/student"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/user"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/user/purchase"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/video"
	vitrine2 "github.com/memberclass-backend-golang/internal/application/handlers/http/vitrine"
	auth2 "github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/application/middlewares/rate_limit"
)

type Router struct {
	chi.Router
	videoHandler              *video.VideoHandler
	lessonHandler             *lesson.LessonHandler
	commentHandler            *comment.CommentHandler
	userActivityHandler       *user.UserActivityHandler
	userPurchaseHandler       *purchase.UserPurchaseHandler
	userInformationsHandler   *user.UserInformationsHandler
	socialCommentHandler      *comment.SocialCommentHandler
	activitySummaryHandler    *user.ActivitySummaryHandler
	lessonsCompletedHandler   *lesson.LessonsCompletedHandler
	studentReportHandler      *student.StudentReportHandler
	swaggerHandler            *internalhttp.SwaggerHandler
	authHandler               *auth.AuthHandler
	ssoHandler                *sso.SSOHandler
	aiLessonHandler           *ai.AILessonHandler
	aiTenantHandler           *ai.AITenantHandler
	vitrineHandler            *vitrine2.VitrineHandler
	rateLimitMiddleware       *rate_limit.RateLimitMiddleware
	rateLimitTenantMiddleware *rate_limit.RateLimitTenantMiddleware
	rateLimitIPMiddleware     *rate_limit.RateLimitIPMiddleware
	authMiddleware            *auth2.AuthMiddleware
	authExternalMiddleware    *auth2.AuthExternalMiddleware
}

func NewRouter(
	videoHandler *video.VideoHandler,
	lessonHandler *lesson.LessonHandler,
	commentHandler *comment.CommentHandler,
	userActivityHandler *user.UserActivityHandler,
	userPurchaseHandler *purchase.UserPurchaseHandler,
	userInformationsHandler *user.UserInformationsHandler,
	socialCommentHandler *comment.SocialCommentHandler,
	activitySummaryHandler *user.ActivitySummaryHandler,
	lessonsCompletedHandler *lesson.LessonsCompletedHandler,
	studentReportHandler *student.StudentReportHandler,
	swaggerHandler *internalhttp.SwaggerHandler,
	authHandler *auth.AuthHandler,
	ssoHandler *sso.SSOHandler,
	aiLessonHandler *ai.AILessonHandler,
	aiTenantHandler *ai.AITenantHandler,
	vitrineHandler *vitrine2.VitrineHandler,
	rateLimitMiddleware *rate_limit.RateLimitMiddleware,
	rateLimitTenantMiddleware *rate_limit.RateLimitTenantMiddleware,
	rateLimitIPMiddleware *rate_limit.RateLimitIPMiddleware,
	authMiddleware *auth2.AuthMiddleware,
	authExternalMiddleware *auth2.AuthExternalMiddleware,
) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	return &Router{
		Router:                    router,
		videoHandler:              videoHandler,
		lessonHandler:             lessonHandler,
		commentHandler:            commentHandler,
		userActivityHandler:       userActivityHandler,
		userPurchaseHandler:       userPurchaseHandler,
		userInformationsHandler:   userInformationsHandler,
		socialCommentHandler:      socialCommentHandler,
		activitySummaryHandler:    activitySummaryHandler,
		lessonsCompletedHandler:   lessonsCompletedHandler,
		studentReportHandler:      studentReportHandler,
		swaggerHandler:            swaggerHandler,
		authHandler:               authHandler,
		ssoHandler:                ssoHandler,
		aiLessonHandler:           aiLessonHandler,
		aiTenantHandler:           aiTenantHandler,
		vitrineHandler:            vitrineHandler,
		rateLimitMiddleware:       rateLimitMiddleware,
		rateLimitTenantMiddleware: rateLimitTenantMiddleware,
		rateLimitIPMiddleware:     rateLimitIPMiddleware,
		authMiddleware:            authMiddleware,
		authExternalMiddleware:    authExternalMiddleware,
	}
}

func (r *Router) SetupRoutes() {
	r.Get("/docs", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/docs/", http.StatusMovedPermanently)
	})
	r.Route("/docs", func(router chi.Router) {
		router.Get("/", r.swaggerHandler.ServeSwaggerUI)
		router.Get("/swagger.yaml", r.swaggerHandler.ServeSwaggerYAML)
	})

	r.Route("/api/v1", func(router chi.Router) {

		router.Route("/auth", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Post("/", r.authHandler.GenerateMagicLink)
		})

		router.Route("/sso", func(router chi.Router) {
			router.With(
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Post("/generate-token", r.ssoHandler.GenerateSSOToken)

			router.Post("/validate-token", r.ssoHandler.ValidateSSOToken)
		})

		router.Route("/ai", func(router chi.Router) {
			router.Route("/lessons", func(router chi.Router) {
				router.Patch("/{lessonId}", r.aiLessonHandler.UpdateTranscriptionStatus)
				router.With(
					r.rateLimitTenantMiddleware.LimitByTenant,
				).Get("/", r.aiLessonHandler.GetLessons)
			})
			router.Route("/tenants", func(router chi.Router) {
				router.Get("/", r.aiTenantHandler.GetTenantsWithAIEnabled)
				router.Post("/process-lessons", r.aiTenantHandler.ProcessLessonsTenant)
			})
		})

		router.Route("/videos", func(router chi.Router) {
			router.With(
				r.rateLimitMiddleware.CheckUploadLimit,
				r.rateLimitMiddleware.IncrementAfterUpload,
			).Post("/upload", r.videoHandler.UploadVideo)
		})

		router.Route("/comments", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/", r.commentHandler.GetComments)
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Patch("/{commentID}", r.commentHandler.UpdateComment)
		})

		router.Route("/user", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
			).Get("/informations", r.userInformationsHandler.GetUserInformations)
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/activities", r.userActivityHandler.GetUserActivities)
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/activity/summary", r.activitySummaryHandler.GetActivitySummary)
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/lessons/completed", r.lessonsCompletedHandler.GetLessonsCompleted)
			router.With(
				r.rateLimitTenantMiddleware.LimitByTenant)
		})

		router.Route("/users", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/purchases", r.userPurchaseHandler.GetUserPurchases)
		})

		router.Route("/social", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Post("/", r.socialCommentHandler.CreateOrUpdatePost)
		})

		router.Route("/student", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/report", r.studentReportHandler.GetStudentReport)
		})

		router.Route("/admin", func(router chi.Router) {
			router.Route("/reports", func(router chi.Router) {
				router.With(
					r.authExternalMiddleware.Authenticate,
					r.rateLimitTenantMiddleware.LimitByTenant,
				).Get("/students-ranking", r.studentReportHandler.GetStudentsRanking)
			})
		})

		router.Route("/vitrine", func(router chi.Router) {
			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/", r.vitrineHandler.GetVitrines)

			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/{vitrineId}", r.vitrineHandler.GetVitrine)

			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/courses/{courseId}", r.vitrineHandler.GetCourse)

			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/modules/{moduleId}", r.vitrineHandler.GetModule)

			router.With(
				r.authExternalMiddleware.Authenticate,
				r.rateLimitTenantMiddleware.LimitByTenant,
			).Get("/lessons/{lessonId}", r.vitrineHandler.GetLesson)
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

		router.Route("/comments", func(router chi.Router) {
			router.With(
				r.authMiddleware.Authenticate).Get("/", r.commentHandler.GetComments)
		})

	})
}
