package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
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
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
)

type Router struct {
	chi.Router
	videoHandler              *video.VideoHandler
	lessonHandler             *lesson.LessonHandler
	commentHandler            *comment.CommentHandler
	userActivities            *user_activities.Feature
	userPurchaseHandler       *purchase.UserPurchaseHandler
	userInformationsHandler   *user.UserInformationsHandler
	socialCommentHandler      *comment.SocialCommentHandler
	activitySummary           *activity_summary.Feature
	memberImport              *member_import.Feature
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
	bearerMiddleware          *auth2.BearerMiddleware
}

func NewRouter(
	videoHandler *video.VideoHandler,
	lessonHandler *lesson.LessonHandler,
	commentHandler *comment.CommentHandler,
	userActivities *user_activities.Feature,
	userPurchaseHandler *purchase.UserPurchaseHandler,
	userInformationsHandler *user.UserInformationsHandler,
	socialCommentHandler *comment.SocialCommentHandler,
	activitySummary *activity_summary.Feature,
	memberImport *member_import.Feature,
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
	bearerMiddleware *auth2.BearerMiddleware,
) *Router {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	// Wildcard CORS works for every route now: public API uses `mc-api-key`
	// and the frontend-only routes (/imports, future /admin/*) use a
	// bearer JWT in `Authorization`. Nothing relies on cookies, so
	// AllowCredentials stays false and `*` is valid.
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Content-Length", "user_id", "mc-api-key", "x-internal-api-key", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	return &Router{
		Router:                    router,
		videoHandler:              videoHandler,
		lessonHandler:             lessonHandler,
		commentHandler:            commentHandler,
		userActivities:            userActivities,
		userPurchaseHandler:       userPurchaseHandler,
		userInformationsHandler:   userInformationsHandler,
		socialCommentHandler:      socialCommentHandler,
		activitySummary:           activitySummary,
		memberImport:              memberImport,
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
		bearerMiddleware:          bearerMiddleware,
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

			router.With(
				r.authExternalMiddleware.Authenticate).Post("/validate-token", r.ssoHandler.ValidateSSOToken)
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

			r.userActivities.Register(router, user_activities.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

			r.activitySummary.Register(router, activity_summary.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

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

	// /imports/* — admin endpoints called from the Next.js frontend using
	// a short-lived Bearer JWT minted by `/api/auth/go-token` on the Next
	// side (same secret: NEXTAUTH_SECRET). Stateless, no cookies.
	// LimitByIP caps abuse of the bulk endpoint when a token leaks or an
	// admin account is compromised — the bearer token alone would otherwise
	// allow unbounded submission of 10k-user batches.
	r.Route("/imports", func(router chi.Router) {
		router.Use(r.rateLimitIPMiddleware.LimitByIP)
		r.memberImport.Register(router, member_import.MiddlewareSet{
			SessionAuth: r.bearerMiddleware.RequireAuth,
		})
	})
}
