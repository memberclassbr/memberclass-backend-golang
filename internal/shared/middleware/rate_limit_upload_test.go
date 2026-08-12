package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memberclass-backend-golang/internal/platform/ratelimit"
)

// fakeUploadLimiter records the key each call was charged against, which is the
// whole point of these tests: the key must come from the verified token and not
// from anything the caller writes.
type fakeUploadLimiter struct {
	result ratelimit.UploadResult
	err    error

	checkedKey  string
	checkedSize int64

	incrementedKey  string
	incrementedSize int64
	incrementCalls  int
}

func (f *fakeUploadLimiter) CheckUploadLimit(_ context.Context, key string, size int64) (ratelimit.UploadResult, error) {
	f.checkedKey, f.checkedSize = key, size
	return f.result, f.err
}

func (f *fakeUploadLimiter) IncrementUploadSize(_ context.Context, key string, size int64) error {
	f.incrementedKey, f.incrementedSize = key, size
	f.incrementCalls++
	return nil
}

func (f *fakeUploadLimiter) GetCurrentUploadSize(context.Context, string) (int64, error) { return 0, nil }

var _ ratelimit.UploadLimiter = (*fakeUploadLimiter)(nil)

func allowingLimiter() *fakeUploadLimiter {
	return &fakeUploadLimiter{result: ratelimit.UploadResult{Allowed: true, MaxSize: 1000}}
}

// uploadRequest builds a POST with a non-zero ContentLength, which the
// middleware requires. `authUser` may be nil to simulate a route mounted
// without RequireAuth above it.
func uploadRequest(authUser *AuthUser) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/videos/upload", strings.NewReader("0123456789"))
	if authUser != nil {
		r = r.WithContext(ContextWithAuthUser(r.Context(), authUser))
	}
	return r
}

func TestCheckUploadLimit_ChargesTheTokenSubject(t *testing.T) {
	limiter := allowingLimiter()
	m := NewRateLimitMiddleware(limiter, fakeLogger{})

	reached := false
	req := uploadRequest(&AuthUser{UserID: "user-from-token", TenantID: "tenant-1"})
	// A caller-supplied header must not influence the key. Before the token
	// carried the identity this header WAS the key, so a client could reset its
	// own quota by rotating the value.
	req.Header.Set("user_id", "user-from-header")

	rec := httptest.NewRecorder()
	m.CheckUploadLimit(okHandler(&reached)).ServeHTTP(rec, req)

	if !reached {
		t.Fatalf("handler was not reached, status %d body %s", rec.Code, rec.Body.String())
	}
	if limiter.checkedKey != "user-from-token" {
		t.Fatalf("quota keyed on %q, want the token subject", limiter.checkedKey)
	}
	if limiter.checkedSize != 10 {
		t.Fatalf("checked size = %d, want 10", limiter.checkedSize)
	}
}

func TestCheckUploadLimit_RejectsWithoutAnAuthenticatedUser(t *testing.T) {
	tests := []struct {
		name string
		user *AuthUser
	}{
		{"no auth user on the context", nil},
		{"auth user with an empty sub", &AuthUser{TenantID: "tenant-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := allowingLimiter()
			m := NewRateLimitMiddleware(limiter, fakeLogger{})

			reached := false
			rec := httptest.NewRecorder()
			m.CheckUploadLimit(okHandler(&reached)).ServeHTTP(rec, uploadRequest(tt.user))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if reached {
				t.Fatal("handler ran without an authenticated caller")
			}
			if limiter.checkedKey != "" {
				t.Fatalf("limiter consulted with key %q, want no call at all", limiter.checkedKey)
			}
		})
	}
}

func TestCheckUploadLimit_RejectsOverQuota(t *testing.T) {
	limiter := &fakeUploadLimiter{result: ratelimit.UploadResult{
		Allowed:       false,
		CurrentSize:   900,
		MaxSize:       1000,
		RemainingSize: 100,
		ResetTime:     1700000000,
	}}
	m := NewRateLimitMiddleware(limiter, fakeLogger{})

	reached := false
	rec := httptest.NewRecorder()
	m.CheckUploadLimit(okHandler(&reached)).ServeHTTP(rec, uploadRequest(&AuthUser{UserID: "u1"}))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if reached {
		t.Fatal("handler ran while the caller was over quota")
	}

	var body struct {
		Data rateLimitError `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the 429 body: %v (%s)", err, rec.Body.String())
	}
	if body.Data.MaxSize != 1000 || body.Data.RemainingSize != 100 {
		t.Fatalf("quota figures missing from the 429 body: %s", rec.Body.String())
	}
}

// serveUploadChain runs the two halves in the order routes.go mounts them, so
// the context handoff between them is what is under test.
func serveUploadChain(limiter ratelimit.UploadLimiter, user *AuthUser, status int) *httptest.ResponseRecorder {
	m := NewRateLimitMiddleware(limiter, fakeLogger{})
	handler := m.CheckUploadLimit(m.IncrementAfterUpload(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) },
	)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, uploadRequest(user))
	return rec
}

func TestIncrementAfterUpload_ChargesTheTokenSubjectOnSuccess(t *testing.T) {
	limiter := allowingLimiter()
	serveUploadChain(limiter, &AuthUser{UserID: "user-from-token"}, http.StatusOK)

	if limiter.incrementCalls != 1 {
		t.Fatalf("increment called %d times, want 1", limiter.incrementCalls)
	}
	if limiter.incrementedKey != "user-from-token" {
		t.Fatalf("charged %q, want the token subject", limiter.incrementedKey)
	}
	if limiter.incrementedSize != 10 {
		t.Fatalf("charged %d bytes, want 10", limiter.incrementedSize)
	}
}

func TestIncrementAfterUpload_DoesNotChargeAFailedUpload(t *testing.T) {
	limiter := allowingLimiter()
	serveUploadChain(limiter, &AuthUser{UserID: "user-from-token"}, http.StatusInternalServerError)

	if limiter.incrementCalls != 0 {
		t.Fatalf("increment called %d times after a 500, want 0", limiter.incrementCalls)
	}
}

// Mounted without CheckUploadLimit above it there is no quota on the context.
// It must stay silent rather than charge some default bucket.
func TestIncrementAfterUpload_NoQuotaOnContextIsANoOp(t *testing.T) {
	limiter := allowingLimiter()
	m := NewRateLimitMiddleware(limiter, fakeLogger{})

	rec := httptest.NewRecorder()
	m.IncrementAfterUpload(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	)).ServeHTTP(rec, uploadRequest(&AuthUser{UserID: "u1"}))

	if limiter.incrementCalls != 0 {
		t.Fatalf("increment called %d times with no quota on the context, want 0", limiter.incrementCalls)
	}
}
