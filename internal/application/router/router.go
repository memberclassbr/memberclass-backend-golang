package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	internalhttp "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/ai"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/auth"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/lesson"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/sso"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/video"
	auth2 "github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/application/middlewares/rate_limit"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	commentfeat "github.com/memberclass-backend-golang/internal/features/api/comment"
	socialfeat "github.com/memberclass-backend-golang/internal/features/api/social"
	studentfeat "github.com/memberclass-backend-golang/internal/features/api/student"
	userfeat "github.com/memberclass-backend-golang/internal/features/api/user"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	vitrinefeat "github.com/memberclass-backend-golang/internal/features/api/vitrine"
	"github.com/memberclass-backend-golang/internal/features/workers/transcription"
)

type Router struct {
	chi.Router
	videoHandler              *video.VideoHandler
	lessonHandler             *lesson.LessonHandler
	comment                   *commentfeat.Feature
	userActivities            *user_activities.Feature
	user                      *userfeat.Feature
	social                    *socialfeat.Feature
	activitySummary           *activity_summary.Feature
	memberImport              *member_import.Feature
	transcription             *transcription.Feature
	student                   *studentfeat.Feature
	swaggerHandler            *internalhttp.SwaggerHandler
	authHandler               *auth.AuthHandler
	ssoHandler                *sso.SSOHandler
	aiLessonHandler           *ai.AILessonHandler
	aiTenantHandler           *ai.AITenantHandler
	vitrine                   *vitrinefeat.Feature
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
	commentFeat *commentfeat.Feature,
	userActivities *user_activities.Feature,
	userFeat *userfeat.Feature,
	socialFeat *socialfeat.Feature,
	activitySummary *activity_summary.Feature,
	memberImport *member_import.Feature,
	transcriptionFeat *transcription.Feature,
	studentFeat *studentfeat.Feature,
	swaggerHandler *internalhttp.SwaggerHandler,
	authHandler *auth.AuthHandler,
	ssoHandler *sso.SSOHandler,
	aiLessonHandler *ai.AILessonHandler,
	aiTenantHandler *ai.AITenantHandler,
	vitrineFeat *vitrinefeat.Feature,
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

	// CORS: echo back the request Origin so the response works for any
	// tenant subdomain / custom domain (multi-tenant). AllowCredentials=true
	// requires a non-wildcard Allow-Origin, so we reflect the caller's Origin
	// instead of sending "*". ExposedHeaders lets the frontend read pagination
	// metadata.
	router.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Content-Length", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	return &Router{
		Router:                    router,
		videoHandler:              videoHandler,
		lessonHandler:             lessonHandler,
		comment:                   commentFeat,
		userActivities:            userActivities,
		user:                      userFeat,
		social:                    socialFeat,
		activitySummary:           activitySummary,
		memberImport:              memberImport,
		transcription:             transcriptionFeat,
		student:                   studentFeat,
		swaggerHandler:            swaggerHandler,
		authHandler:               authHandler,
		ssoHandler:                ssoHandler,
		aiLessonHandler:           aiLessonHandler,
		aiTenantHandler:           aiTenantHandler,
		vitrine:                   vitrineFeat,
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
				// /lessons/{lessonId}/transcription PATCH lives in the
				// transcription slice; keep the GET / endpoint here for
				// the AI dashboard (paginated lessons listing).
				router.With(
					r.rateLimitTenantMiddleware.LimitByTenant,
				).Get("/", r.aiLessonHandler.GetLessons)
			})
			router.Route("/tenants", func(router chi.Router) {
				router.Get("/", r.aiTenantHandler.GetTenantsWithAIEnabled)
				// process-lessons now flows through the transcription
				// slice (registered below) — old handler removed.
			})

			// Transcription slice owns:
			//   POST  /tenants/process-lessons
			//   GET   /jobs/{jobId}
			//   PATCH /lessons/{lessonId}/transcription
			r.transcription.Register(router, transcription.MiddlewareSet{})
		})

		router.Route("/videos", func(router chi.Router) {
			router.With(
				r.rateLimitMiddleware.CheckUploadLimit,
				r.rateLimitMiddleware.IncrementAfterUpload,
			).Post("/upload", r.videoHandler.UploadVideo)
		})

		router.Route("/comments", func(router chi.Router) {
			r.comment.Register(router, commentfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/user", func(router chi.Router) {
			r.user.Register(router, userfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

			r.userActivities.Register(router, user_activities.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

			r.activitySummary.Register(router, activity_summary.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

		})

		router.Route("/users", func(router chi.Router) {
			r.user.RegisterUsers(router, userfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/social", func(router chi.Router) {
			r.social.Register(router, socialfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/student", func(router chi.Router) {
			r.student.Register(router, studentfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/vitrine", func(router chi.Router) {
			r.vitrine.Register(router, vitrinefeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
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
			r.comment.RegisterLegacy(router, commentfeat.MiddlewareSet{
				AuthAPIKey: r.authMiddleware.Authenticate,
			})
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
