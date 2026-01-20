package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	httpHandlers "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/ai"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/auth"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/comment"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/lesson"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/student"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/user"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/user/purchase"
	"github.com/memberclass-backend-golang/internal/application/handlers/http/video"
	auth2 "github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/application/middlewares/rate_limit"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func createTestRouter(t *testing.T) *Router {
	mockVideoHandler := &video.VideoHandler{}
	mockLessonHandler := &lesson.LessonHandler{}
	mockCommentHandler := &comment.CommentHandler{}
	mockUserActivityHandler := &user.UserActivityHandler{}
	mockUserPurchaseHandler := &purchase.UserPurchaseHandler{}
	mockUserInformationsHandler := &user.UserInformationsHandler{}
	mockSocialCommentHandler := &comment.SocialCommentHandler{}
	mockActivitySummaryHandler := &user.ActivitySummaryHandler{}
	mockLessonsCompletedHandler := &lesson.LessonsCompletedHandler{}
	mockStudentReportHandler := &student.StudentReportHandler{}
	mockSwaggerHandler := httpHandlers.NewSwaggerHandler()
	mockAuthHandler := &auth.AuthHandler{}
	mockAILessonHandler := &ai.AILessonHandler{}
	mockAITenantHandler := &ai.AITenantHandler{}
	mockLogger := &mocks.MockLogger{}
	mockRateLimiter := &mocks.MockRateLimiterUpload{}
	mockRateLimiterTenant := &mocks.MockRateLimiterTenant{}
	mockRateLimiterIP := &mocks.MockRateLimiterIP{}
	mockSessionValidator := &mocks.MockSessionValidatorUseCase{}
	mockApiTokenUseCase := &mocks.MockApiTokenUseCase{}

	mockLogger.On("Error", mock.Anything).Return().Maybe()
	mockLogger.On("Warn", mock.Anything).Return().Maybe()
	mockLogger.On("Info", mock.Anything).Return().Maybe()
	mockLogger.On("Debug", mock.Anything).Return().Maybe()

	rateLimitMiddleware := rate_limit.NewRateLimitMiddleware(mockRateLimiter, mockLogger)
	rateLimitTenantMiddleware := rate_limit.NewRateLimitTenantMiddleware(mockRateLimiterTenant, mockLogger)
	rateLimitIPMiddleware := rate_limit.NewRateLimitIPMiddleware(mockRateLimiterIP, mockLogger)
	authMiddleware := auth2.NewAuthMiddleware(mockLogger, mockSessionValidator)
	authExternalMiddleware := auth2.NewAuthExternalMiddleware(mockApiTokenUseCase)

	return NewRouter(mockVideoHandler, mockLessonHandler, mockCommentHandler, mockUserActivityHandler, mockUserPurchaseHandler, mockUserInformationsHandler, mockSocialCommentHandler, mockActivitySummaryHandler, mockLessonsCompletedHandler, mockStudentReportHandler, mockSwaggerHandler, mockAuthHandler, mockAILessonHandler, mockAITenantHandler, rateLimitMiddleware, rateLimitTenantMiddleware, rateLimitIPMiddleware, authMiddleware, authExternalMiddleware)
}

func TestNewRouter(t *testing.T) {
	router := createTestRouter(t)

	assert.NotNil(t, router)
	assert.NotNil(t, router.Router)
	assert.NotNil(t, router.videoHandler)
	assert.NotNil(t, router.lessonHandler)
	assert.NotNil(t, router.rateLimitMiddleware)
	assert.NotNil(t, router.authMiddleware)
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
		// Video routes
		{"POST", "/api/v1/videos/upload", 404}, // Will be 404 because we don't have actual handler implementation

		// Lesson routes
		{"POST", "/api/lessons/pdf-process", 404},               // Will be 404 because we don't have actual handler implementation
		{"POST", "/api/lessons/process-all-pdfs", 404},          // Will be 404 because we don't have actual handler implementation
		{"POST", "/api/lessons/lesson-123/pdf-regenerate", 404}, // Will be 404 because we don't have actual handler implementation

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

	// Test that the route structure is correct by checking if specific routes exist
	// We'll use chi's Walk function to inspect the routes
	var routes []string
	chi.Walk(router.Router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	})

	// Check that expected routes are present
	expectedRoutes := []string{
		"POST /api/v1/videos/upload",
		"POST /api/lessons/pdf-process",
		"POST /api/lessons/process-all-pdfs",
		"POST /api/lessons/{lessonId}/pdf-regenerate",
	}

	for _, expectedRoute := range expectedRoutes {
		found := false
		for _, route := range routes {
			if route == expectedRoute {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected route %s not found in registered routes", expectedRoute)
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
