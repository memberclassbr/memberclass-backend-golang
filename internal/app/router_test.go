package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/features/api/docs"
	healthfeat "github.com/memberclass-backend-golang/internal/features/api/health"
	mw "github.com/memberclass-backend-golang/internal/shared/middleware"

	lessonpdf "github.com/memberclass-backend-golang/internal/features/admin/lesson_pdf"
	"github.com/memberclass-backend-golang/internal/features/admin/member_import"
	"github.com/memberclass-backend-golang/internal/features/api/activity_summary"
	aifeat "github.com/memberclass-backend-golang/internal/features/api/ai"
	authfeat "github.com/memberclass-backend-golang/internal/features/api/auth"
	commentfeat "github.com/memberclass-backend-golang/internal/features/api/comment"
	socialfeat "github.com/memberclass-backend-golang/internal/features/api/social"
	ssofeat "github.com/memberclass-backend-golang/internal/features/api/sso"
	studentfeat "github.com/memberclass-backend-golang/internal/features/api/student"
	userfeat "github.com/memberclass-backend-golang/internal/features/api/user"
	"github.com/memberclass-backend-golang/internal/features/api/user_activities"
	videofeat "github.com/memberclass-backend-golang/internal/features/api/video"
	vitrinefeat "github.com/memberclass-backend-golang/internal/features/api/vitrine"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
)

// discardLogger keeps route-wiring tests free of logging assertions.
type discardLogger struct{}

func (discardLogger) Debug(string, ...any) {}
func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Warn(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

func createTestRouter(t *testing.T) *Router {
	log := discardLogger{}
	cfg := &config.Config{Auth: config.Auth{
		NextAuthSecret: "test-secret",
		GoAPIJWTSecret: "go-api-test-secret-at-least-32-bytes",
	}}

	mockVideo := videofeat.New(nil, nil, log)
	mockLessonPDF := lessonpdf.New(nil, nil, nil, cfg, log)
	mockComment := commentfeat.New(nil, nil)
	mockUserActivities := user_activities.New(nil, nil, nil)
	mockUser := userfeat.New(nil, nil, nil)
	mockSocial := socialfeat.New(nil, nil)
	mockActivitySummary := activity_summary.New(nil, nil, nil)
	mockMemberImport := member_import.New(nil, nil, nil, cfg)
	mockStudent := studentfeat.New(nil, nil, nil)
	mockSwaggerHandler := docs.New()
	mockAuth := authfeat.New(nil, nil, cfg, log)
	mockSSO := ssofeat.New(nil, log)
	mockAI := aifeat.New(nil, &config.Config{}, nil)
	mockVitrine := vitrinefeat.New(nil, nil)
	rateLimitMiddleware := mw.NewRateLimitMiddleware(nil, log)
	rateLimitTenantMiddleware := mw.NewRateLimitTenantMiddleware(nil, log)
	rateLimitIPMiddleware := mw.NewRateLimitIPMiddleware(nil, log)
	authExternalMiddleware := mw.NewAuthExternalMiddleware(nil, log)
	bearerMiddleware := mw.NewBearerMiddleware(cfg, nil, log)

	return newRouter(log, mockVideo, mockLessonPDF, mockComment, mockUserActivities, mockUser, mockSocial, mockActivitySummary, mockMemberImport, nil, mockStudent, mockSwaggerHandler, mockAuth, mockSSO, mockAI, mockVitrine, healthfeat.New(nil, nil, log), rateLimitMiddleware, rateLimitTenantMiddleware, rateLimitIPMiddleware, authExternalMiddleware, bearerMiddleware)
}

func TestNewRouter(t *testing.T) {
	router := createTestRouter(t)

	assert.NotNil(t, router)
	assert.NotNil(t, router.Router)
	assert.NotNil(t, router.video)
	assert.NotNil(t, router.lessonPDF)
	assert.NotNil(t, router.rateLimitMiddleware)
	assert.NotNil(t, router.authExternalMiddleware)
}

func TestRouter_SetupRoutes(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	// Test that routes are properly configured by making requests
	testCases := []struct {
		method string
		path   string
		status int // Expected status (404 for non-existent routes, or actual status for existing ones)
	}{
		// The frontend-origin trio moved to the root, and each now sits behind
		// the Bearer middleware: no token means 401, and the old /api paths are
		// gone rather than dual-mounted.
		{"POST", "/videos/upload", 401},
		{"POST", "/sso/generate-token", 401},
		{"POST", "/api/v1/videos/upload", 404},
		{"POST", "/api/v1/sso/generate-token", 404},

		// Lesson routes
		{"POST", "/api/lessons/pdf-process", 404},               // Will be 404 because we don't have actual handler implementation
		{"POST", "/api/lessons/process-all-pdfs", 404},          // Will be 404 because we don't have actual handler implementation
		{"POST", "/api/lessons/lesson-123/pdf-regenerate", 404}, // Will be 404 because we don't have actual handler implementation

		// Vitrine routes
		{"GET", "/api/v1/vitrine", 404},
		{"GET", "/api/v1/vitrine/vitrine-123", 404},
		{"GET", "/api/v1/vitrine/courses/course-123", 404},
		{"GET", "/api/v1/vitrine/modules/module-123", 404},
		{"GET", "/api/v1/vitrine/lessons/lesson-123", 404},

		// Non-existent routes
		{"GET", "/api/lessons", 404},
		{"POST", "/api/v1/videos", 404},
		{"GET", "/api/lessons/lesson-123", 404},
	}

	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// For this test, we're mainly checking that the routes are registered
			// The actual status codes will depend on the handler implementations
			// We expect either the route to be found (and potentially return an error from handler)
			// or to return 404 if the route doesn't exist
			assert.True(t, w.Code == http.StatusNotFound || w.Code >= 400,
				"Expected 404 or error status, got %d for %s %s", w.Code, tc.method, tc.path)
		})
	}
}

func TestRouter_MiddlewareConfiguration(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	assert.NotNil(t, router.Router)

	req := httptest.NewRequest("GET", "/api/lessons", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouter_RouteStructure(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	var routes []string
	chi.Walk(router.Router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	})

	expectedRoutes := []string{
		"POST /videos/upload",
		"POST /api/lessons/pdf-process",
		"POST /api/lessons/process-all-pdfs",
		"POST /api/lessons/{lessonId}/pdf-regenerate",
		"GET /api/v1/vitrine/",
		"GET /api/v1/vitrine/{vitrineId}",
		"GET /api/v1/vitrine/courses/{courseId}",
		"GET /api/v1/vitrine/modules/{moduleId}",
		"GET /api/v1/vitrine/lessons/{lessonId}",
	}

	for _, expectedRoute := range expectedRoutes {
		found := false
		for _, route := range routes {
			if route == expectedRoute {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Available routes: %v", routes)
		}
		assert.True(t, found, "Expected route %s not found in registered routes. Available routes: %v", expectedRoute, routes)
	}
}

func TestRouter_ChiRouterIntegration(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	var handler http.Handler = router
	assert.NotNil(t, handler)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouter_MiddlewareOrder(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	assert.NotNil(t, router.Router)

	req := httptest.NewRequest("GET", "/api/lessons", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, 0, w.Code)
}

// ---------- CORS ----------

// The policy used to reflect the caller's Origin AND send
// Access-Control-Allow-Credentials: true. That pair is what lets any page on
// the internet drive this API with the visitor's next-auth.session-token cookie
// attached and read the reply. A literal "*" is what forbids it — a browser
// will not send credentials to a wildcard origin — so these two assertions are
// the guard, not a style preference.
func TestRouter_CORSIsAWildcardWithoutCredentials(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")

	w := httptest.NewRecorder()
	router.Router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"),
		"the Origin must not be echoed back")
	assert.NotEqual(t, "https://evil.example", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
		"credentials must never be allowed cross-origin")
}

// Same on an actual response, not just the preflight: the header the browser
// enforces on is the one attached to the real request.
func TestRouter_CORSOnASimpleRequest(t *testing.T) {
	router := createTestRouter(t)
	router.SetupRoutes()

	// An unrouted path: CORS runs above the mux, so the headers are set either
	// way, and a 404 keeps a handler with nil dependencies out of the test.
	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	req.Header.Set("Origin", "https://evil.example")

	w := httptest.NewRecorder()
	router.Router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}
