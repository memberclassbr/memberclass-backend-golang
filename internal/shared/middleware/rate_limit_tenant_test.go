package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// recordingTenantLimiter remembers which bucket the middleware charged.
type recordingTenantLimiter struct {
	checked   []string
	increased []string
}

func (l *recordingTenantLimiter) CheckLimit(_ context.Context, tenantID, _ string) (bool, ratelimit.Info, error) {
	l.checked = append(l.checked, tenantID)
	return true, ratelimit.Info{Limit: 10, Remaining: 9}, nil
}

func (l *recordingTenantLimiter) Increment(_ context.Context, tenantID, _ string) error {
	l.increased = append(l.increased, tenantID)
	return nil
}

var _ ratelimit.TenantLimiter = (*recordingTenantLimiter)(nil)

// The quota is per area, and the area is whatever the credential resolved to.
// Reading ?tenantId= first — which is what this did — let a caller with a valid
// key move itself to an empty bucket on every request just by changing the
// value, which is not a rate limit at all.
func TestLimitByTenant_IgnoresTheQueryStringWhenAuthenticated(t *testing.T) {
	limiter := &recordingTenantLimiter{}
	m := NewRateLimitTenantMiddleware(limiter, fakeLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user?tenantId=someone-else", nil)
	req = req.WithContext(tenant.NewContext(req.Context(), &tenant.Tenant{ID: "t1", Name: "Acme"}))

	var reached bool
	m.LimitByTenant(okHandler(&reached)).ServeHTTP(httptest.NewRecorder(), req)

	require.True(t, reached)
	assert.Equal(t, []string{"t1"}, limiter.checked)
	assert.Equal(t, []string{"t1"}, limiter.increased)
}

// Routes mounted without AuthExternal have no tenant in context. They still get
// a bucket, because the alternative is no limit at all.
func TestLimitByTenant_FallsBackToTheQueryStringWhenUnauthenticated(t *testing.T) {
	limiter := &recordingTenantLimiter{}
	m := NewRateLimitTenantMiddleware(limiter, fakeLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/summary?tenantId=t2", nil)

	var reached bool
	m.LimitByTenant(okHandler(&reached)).ServeHTTP(httptest.NewRecorder(), req)

	require.True(t, reached)
	assert.Equal(t, []string{"t2"}, limiter.checked)
}

// With neither a tenant nor a query parameter there is nothing to key on, and
// the request passes rather than being refused for it.
func TestLimitByTenant_PassesThroughWithNoTenantAtAll(t *testing.T) {
	limiter := &recordingTenantLimiter{}
	m := NewRateLimitTenantMiddleware(limiter, fakeLogger{})

	var reached bool
	m.LimitByTenant(okHandler(&reached)).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	require.True(t, reached)
	assert.Empty(t, limiter.checked)
}
