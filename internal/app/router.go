package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/riandyrn/otelchi"
	otelchimetric "github.com/riandyrn/otelchi/metric"

	"github.com/memberclass-backend-golang/internal/features/api/docs"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"
	mw "github.com/memberclass-backend-golang/internal/shared/middleware"

	lessonpdf "github.com/memberclass-backend-golang/internal/features/admin/lesson_pdf"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	aifeat "github.com/memberclass-backend-golang/internal/features/api/ai"
	authfeat "github.com/memberclass-backend-golang/internal/features/api/auth"
	commentfeat "github.com/memberclass-backend-golang/internal/features/api/comment"
	healthfeat "github.com/memberclass-backend-golang/internal/features/api/health"
	socialfeat "github.com/memberclass-backend-golang/internal/features/api/social"
	ssofeat "github.com/memberclass-backend-golang/internal/features/api/sso"
	studentfeat "github.com/memberclass-backend-golang/internal/features/api/student"
	userfeat "github.com/memberclass-backend-golang/internal/features/api/user"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	videofeat "github.com/memberclass-backend-golang/internal/features/api/video"
	vitrinefeat "github.com/memberclass-backend-golang/internal/features/api/vitrine"
	"github.com/memberclass-backend-golang/internal/features/workers/transcription"
)

type Router struct {
	chi.Router
	video                     *videofeat.Feature
	lessonPDF                 *lessonpdf.Feature
	comment                   *commentfeat.Feature
	userActivities            *user_activities.Feature
	user                      *userfeat.Feature
	social                    *socialfeat.Feature
	activitySummary           *activity_summary.Feature
	memberImport              *member_import.Feature
	transcription             *transcription.Feature
	student                   *studentfeat.Feature
	docs                      *docs.Feature
	auth                      *authfeat.Feature
	sso                       *ssofeat.Feature
	ai                        *aifeat.Feature
	vitrine                   *vitrinefeat.Feature
	health                    *healthfeat.Feature
	rateLimitMiddleware       *mw.RateLimitMiddleware
	rateLimitTenantMiddleware *mw.RateLimitTenantMiddleware
	rateLimitIPMiddleware     *mw.RateLimitIPMiddleware
	authMiddleware            *mw.AuthMiddleware
	authExternalMiddleware    *mw.AuthExternalMiddleware
	bearerMiddleware          *mw.BearerMiddleware
}

func newRouter(
	log logger.Logger,
	videoFeat *videofeat.Feature,
	lessonPDFFeat *lessonpdf.Feature,
	commentFeat *commentfeat.Feature,
	userActivities *user_activities.Feature,
	userFeat *userfeat.Feature,
	socialFeat *socialfeat.Feature,
	activitySummary *activity_summary.Feature,
	memberImport *member_import.Feature,
	transcriptionFeat *transcription.Feature,
	studentFeat *studentfeat.Feature,
	docsFeat *docs.Feature,
	authFeat *authfeat.Feature,
	ssoFeat *ssofeat.Feature,
	aiFeat *aifeat.Feature,
	vitrineFeat *vitrinefeat.Feature,
	healthFeat *healthfeat.Feature,
	rateLimitMiddleware *mw.RateLimitMiddleware,
	rateLimitTenantMiddleware *mw.RateLimitTenantMiddleware,
	rateLimitIPMiddleware *mw.RateLimitIPMiddleware,
	authMiddleware *mw.AuthMiddleware,
	authExternalMiddleware *mw.AuthExternalMiddleware,
	bearerMiddleware *mw.BearerMiddleware,
) *Router {
	router := chi.NewRouter()

	// Order matters and used to be wrong. RequestID and RealIP put values in
	// the context that everything downstream reads, so they go first — with the
	// logger ahead of them, as it was, every line was written without a request
	// id and with the platform proxy's address instead of the caller's.
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	// OTel sits above auth on purpose: a 401 is operational signal, and a
	// middleware that short-circuits before this one would make those requests
	// invisible. WithChiRoutes hands the middleware the router so it can label
	// spans with the route pattern (/api/v1/vitrine/{vitrineId}) instead of the
	// raw path — one time series per endpoint rather than one per id. The
	// routes are registered later, in SetupRoutes; the middleware resolves the
	// pattern per request, so it sees them.
	router.Use(otelchi.Middleware(telemetry.ServiceName, otelchi.WithChiRoutes(router)))

	// The Server* names replace NewRequestDurationMillis / NewRequestInFlight /
	// NewResponseSizeBytes, which otelchi deprecated in favour of the semconv
	// instrument names. Request duration carries the status code as a label, so
	// request counts per status come out of it without a separate counter.
	metricCfg := otelchimetric.NewBaseConfig(telemetry.ServiceName)
	router.Use(
		otelchimetric.NewServerRequestDuration(metricCfg),
		otelchimetric.NewServerActiveRequests(metricCfg),
		otelchimetric.NewServerResponseBodySize(metricCfg),
	)

	router.Use(mw.RequestLogger(log))

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
		video:                     videoFeat,
		lessonPDF:                 lessonPDFFeat,
		comment:                   commentFeat,
		userActivities:            userActivities,
		user:                      userFeat,
		social:                    socialFeat,
		activitySummary:           activitySummary,
		memberImport:              memberImport,
		transcription:             transcriptionFeat,
		student:                   studentFeat,
		docs:                      docsFeat,
		auth:                      authFeat,
		sso:                       ssoFeat,
		ai:                        aiFeat,
		vitrine:                   vitrineFeat,
		health:                    healthFeat,
		rateLimitMiddleware:       rateLimitMiddleware,
		rateLimitTenantMiddleware: rateLimitTenantMiddleware,
		rateLimitIPMiddleware:     rateLimitIPMiddleware,
		authMiddleware:            authMiddleware,
		authExternalMiddleware:    authExternalMiddleware,
		bearerMiddleware:          bearerMiddleware,
	}
}

func (r *Router) SetupRoutes() {
	// Mounted at the root, unguarded, so the platform's healthcheck can reach
	// it without a credential. See the health package comment.
	r.health.Register(r.Router, healthfeat.MiddlewareSet{})

	r.Get("/docs", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/docs/", http.StatusMovedPermanently)
	})
	r.Route("/docs", func(router chi.Router) {
		r.docs.Register(router, docs.MiddlewareSet{})
	})

	r.Route("/api/v1", func(router chi.Router) {

		router.Route("/auth", func(router chi.Router) {
			r.auth.Register(router, authfeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/sso", func(router chi.Router) {
			r.sso.Register(router, ssofeat.MiddlewareSet{
				AuthExternal:    r.authExternalMiddleware.Authenticate,
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})
		})

		router.Route("/ai", func(router chi.Router) {
			// The dashboard's read endpoints.
			r.ai.Register(router, aifeat.MiddlewareSet{
				RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
			})

			// Transcription owns the rest of this prefix:
			//   POST  /tenants/process-lessons
			//   GET   /jobs/{jobId}
			//   PATCH /lessons/{lessonId}/transcription
			r.transcription.Register(router, transcription.MiddlewareSet{})
		})

		router.Route("/videos", func(router chi.Router) {
			r.video.Register(router, videofeat.MiddlewareSet{
				CheckUploadLimit:     r.rateLimitMiddleware.CheckUploadLimit,
				IncrementAfterUpload: r.rateLimitMiddleware.IncrementAfterUpload,
			})
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
			r.lessonPDF.Register(router, lessonpdf.MiddlewareSet{})
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
