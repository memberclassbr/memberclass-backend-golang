package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeCache struct{ err error }

func (f fakeCache) Get(context.Context, string) (string, error) { return "", f.err }
func (f fakeCache) Set(context.Context, string, string, time.Duration) error {
	return f.err
}
func (f fakeCache) Increment(context.Context, string, int64) (int64, error) { return 0, f.err }
func (f fakeCache) Delete(context.Context, string) error                    { return f.err }
func (f fakeCache) Exists(context.Context, string) (bool, error)            { return false, f.err }
func (f fakeCache) TTL(context.Context, string) (time.Duration, error)      { return 0, f.err }
func (f fakeCache) Close() error                                            { return f.err }

type nopLogger struct{}

func (nopLogger) Debug(string, ...any) {}
func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

func TestCheck_AllDependenciesUp(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectPing()

	rec := httptest.NewRecorder()
	New(db, fakeCache{}, nopLogger{}).Check(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body checkResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store: a cached health check reports a past state", got)
	}
}

func TestCheck_DatabaseDown(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectPing().WillReturnError(errors.New("dial tcp 10.0.0.1:5432: connect: connection refused"))

	rec := httptest.NewRecorder()
	New(db, fakeCache{}, nopLogger{}).Check(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	// The route has no credential. Leaking the driver error would hand an
	// anonymous caller the database host and port.
	if body := rec.Body.String(); strings.Contains(body, "10.0.0.1") || strings.Contains(body, "connection refused") {
		t.Errorf("response body leaks infrastructure detail: %s", body)
	}
}

func TestCheck_CacheDown(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectPing()

	rec := httptest.NewRecorder()
	feature := New(db, fakeCache{err: errors.New("redis: connection pool timeout")}, nopLogger{})
	feature.Check(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestFailedDependencies_ReportsBothNotJustTheFirst(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectPing().WillReturnError(errors.New("db down"))

	feature := New(db, fakeCache{err: errors.New("cache down")}, nopLogger{})
	failed := feature.failedDependencies(context.Background())

	if len(failed) != 2 {
		t.Fatalf("got %d failures, want 2 — stopping at the first sends the operator chasing one outage while two are live", len(failed))
	}
}

func TestCheck_RejectsNonGET(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil, fakeCache{}, nopLogger{}).Check(rec, httptest.NewRequest(http.MethodPost, "/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
