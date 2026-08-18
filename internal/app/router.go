package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/riandyrn/otelchi"

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

	// OTel sits above auth on purpose: a 401 is operational signal, and a
	// middleware that short-circuits before this one would make those requests
	// invisible. WithChiRoutes hands the middleware the router so it can label
	// spans with the route pattern (/api/v1/vitrine/{vitrineId}) instead of the
	// raw path — one time series per endpoint rather than one per id. The
	// routes are registered later, in SetupRoutes; the middleware resolves the
	// pattern per request, so it sees them.
	//
	// The filter is the same predicate the metrics middleware below uses, so
	// the two families measure the same set of requests. See
	// telemetry.Instrumented.
	router.Use(otelchi.Middleware(telemetry.ServiceName,
		otelchi.WithChiRoutes(router),
		otelchi.WithFilter(telemetry.Instrumented),
	))

	// Server metrics come from telemetry.HTTPServerMetrics rather than
	// otelchi's metric middlewares. Those recorded no status code — the
	// service had a latency histogram and no way to compute an error rate —
	// and labelled everything with the pre-1.21 attribute names
	// (http.method rather than http.request.method), which no current
	// dashboard or semconv-based alert matches.
	//
	// It goes *inside* the tracing middleware so the histograms are recorded
	// with a live span in context: that is what puts trace exemplars on the
	// latency buckets, which is the jump from "p99 is bad" to the trace that
	// was the p99.
	router.Use(telemetry.HTTPServerMetrics)

	// Recoverer sits below both, and that is a change: above them, a handler
	// panic unwound straight past the instrumentation and the resulting 500
	// was recorded by nothing at all. Here the 500 it writes is an ordinary
	// response as far as the metrics are concerned.
	router.Use(middleware.Recoverer)

	router.Use(mw.RequestLogger(log))

	// CORS is a literal "*", and the two things it does NOT do are the point.
	//
	// It does not reflect the caller's Origin, and it does not set
	// Allow-Credentials. Those two together — which is what this used to send —
	// are the combination browsers treat as "every site on the internet may
	// make an authenticated cross-origin request here and read the reply". The
	// route that made that concrete was `GET /api/comments`, authenticated by
	// the next-auth.session-token cookie: any page could have driven it with a
	// visitor's session attached and read the response back.
	//
	// That route is gone and this is a wildcard, and the two belong together.
	// A literal "*" is what makes credentialed cross-origin requests impossible
	// rather than merely unattractive — a browser will not send credentials to
	// a wildcard origin at all. The wildcard is the enforcement here, not a
	// relaxation.
	//
	// It stays a wildcard rather than an allowlist because tenants bring their
	// own custom domains and the set is not enumerable from this deployment.
	// Nothing is given away by that now: every credential this service accepts
	// travels in a header a browser will not attach on its own — Bearer,
	// x-api-key, x-internal-api-key — so a cross-origin request from an
	// attacker's page arrives unauthenticated, which is a request from nobody.
	//
	// A route that ever needs to be callable cross-origin from a browser *with*
	// a credential gets a header credential. This does not get widened back.
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Content-Length", "X-Total-Count"},
		AllowCredentials: false,
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

		// Only validate-token remains here: it is called by the tenant's own
		// site with the tenant API key. generate-token moved to /sso.
		router.Route("/sso", func(router chi.Router) {
			r.sso.RegisterTenantAPI(router, ssofeat.MiddlewareSet{
				AuthExternal: r.authExternalMiddleware.Authenticate,
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
	})

	// ---- Frontend-origin surface, mounted at the root without /api ----
	//
	// These three are called from the Next.js frontend with a short-lived
	// Bearer JWT minted by `/api/auth/go-token` on the Next side (same secret:
	// NEXTAUTH_SECRET). Stateless, no cookies.
	//
	// The Bearer only establishes *who* is calling. It carries a `role` claim
	// and names no tenant, so every one of these handlers goes on to read the
	// caller's role for the tenant in the request out of "UsersOnTenants" —
	// see internal/shared/tenantrole. Which roles pass differs per route and
	// is declared at the call site, not here.

	// LimitByIP caps abuse of the bulk endpoint when a token leaks or an
	// admin account is compromised — the bearer token alone would otherwise
	// allow unbounded submission of 10k-user batches.
	r.Route("/imports", func(router chi.Router) {
		router.Use(r.rateLimitIPMiddleware.LimitByIP)
		r.memberImport.Register(router, member_import.MiddlewareSet{
			BearerAuth: r.bearerMiddleware.RequireAuth,
		})
	})

	r.Route("/sso", func(router chi.Router) {
		r.sso.Register(router, ssofeat.MiddlewareSet{
			BearerAuth:      r.bearerMiddleware.RequireAuth,
			RateLimitTenant: r.rateLimitTenantMiddleware.LimitByTenant,
		})
	})

	r.Route("/videos", func(router chi.Router) {
		r.video.Register(router, videofeat.MiddlewareSet{
			BearerAuth:           r.bearerMiddleware.RequireAuth,
			CheckUploadLimit:     r.rateLimitMiddleware.CheckUploadLimit,
			IncrementAfterUpload: r.rateLimitMiddleware.IncrementAfterUpload,
		})
	})
}
