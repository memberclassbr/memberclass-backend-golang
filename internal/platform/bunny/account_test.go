package bunny

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

type quietLogger struct{}

func (quietLogger) Debug(string, ...any) {}
func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}

var _ logger.Logger = quietLogger{}

// newAccountClient points the client at a test server and pins its clock, so
// the retention check does not depend on when the suite runs.
func newAccountClient(t *testing.T, handler http.HandlerFunc) *accountClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := &config.Config{}
	cfg.Bunny.APIKey = "account-key"
	cfg.Bunny.AccountBaseURL = server.URL
	cfg.Bunny.Timeout = 5 * time.Second

	client := NewAccountService(cfg, quietLogger{}).(*accountClient)
	client.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	return client
}

func TestGetVideoLibrary_ReadsUsageAndPullZone(t *testing.T) {
	var gotPath, gotKey string
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("AccessKey")
		w.Write([]byte(`{"Id":42,"Name":"area","PullZoneId":3697175,"StorageUsage":123,"TrafficUsage":456}`))
	})

	library, err := client.GetVideoLibrary(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetVideoLibrary: %v", err)
	}

	if gotPath != "/videolibrary/42" {
		t.Errorf("path = %q, want /videolibrary/42", gotPath)
	}
	// The account key, not a per-library one: this endpoint is on a different
	// host and a library key cannot reach it.
	if gotKey != "account-key" {
		t.Errorf("AccessKey = %q, want the account key", gotKey)
	}
	if library.PullZoneID != 3697175 || library.StorageUsage != 123 || library.TrafficUsage != 456 {
		t.Errorf("library = %+v", library)
	}
}

// A 404 has to be its own error: the caller records it as unknown usage rather
// than as zero, and must not count it as a failure worth alerting on.
func TestGetVideoLibrary_404IsLibraryNotFound(t *testing.T) {
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := client.GetVideoLibrary(context.Background(), "gone")
	if !errors.Is(err, ErrLibraryNotFound) {
		t.Fatalf("err = %v, want ErrLibraryNotFound", err)
	}
}

// A rejected key fails every area at once, so it is distinguishable and the
// caller stops rather than repeating it a hundred times.
func TestGetVideoLibrary_401And403AreUnauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})

		if _, err := client.GetVideoLibrary(context.Background(), "42"); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("status %d: err = %v, want ErrUnauthorized", status, err)
		}
	}
}

func TestGetStatistics_SendsTheClosedPeriod(t *testing.T) {
	var gotQuery map[string]string
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = map[string]string{}
		for key, values := range r.URL.Query() {
			gotQuery[key] = values[0]
		}
		w.Write([]byte(`{"TotalBandwidthUsed":129377181}`))
	})

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	stats, err := client.GetStatistics(context.Background(), "3697175", from, from.AddDate(0, 1, -1))
	if err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}

	// dateTo is the last day of the month, inclusive — the period the month's
	// bill is read from, not the reset-prone library counter.
	want := map[string]string{"pullZone": "3697175", "dateFrom": "2026-08-01", "dateTo": "2026-08-31"}
	for key, value := range want {
		if gotQuery[key] != value {
			t.Errorf("query %s = %q, want %q", key, gotQuery[key], value)
		}
	}
	if stats.TotalBandwidthUsed != 129377181 {
		t.Errorf("TotalBandwidthUsed = %d", stats.TotalBandwidthUsed)
	}
}

// Both limits are enforced before the request, so a caller learns "Bunny does
// not answer for this" without spending a call to find out — and can tell it
// apart from a call that failed, which is the difference between leaving no row
// and retrying.
func TestGetStatistics_RefusesRangesTheAPIWillNot(t *testing.T) {
	called := false
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"TotalBandwidthUsed":0}`))
	})

	cases := map[string]struct{ from, to time.Time }{
		"wider than 40 days": {
			from: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			to:   time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		},
		"older than a year": {
			from: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			to:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
		},
		"backwards": {
			from: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
			to:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for name, tc := range cases {
		if _, err := client.GetStatistics(context.Background(), "1", tc.from, tc.to); !errors.Is(err, ErrStatisticsOutOfRange) {
			t.Errorf("%s: err = %v, want ErrStatisticsOutOfRange", name, err)
		}
	}
	if called {
		t.Error("a range the API refuses should not cost a request")
	}
}

// A calendar month is the largest unit the 40-day window allows, and every one
// of them has to fit — including a 31-day month at the edge.
func TestGetStatistics_AcceptsAFullCalendarMonth(t *testing.T) {
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"TotalBandwidthUsed":1}`))
	})

	for month := 1; month <= 12; month++ {
		from := time.Date(2026, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		client.now = func() time.Time { return from.AddDate(0, 1, 0) }

		if _, err := client.GetStatistics(context.Background(), "1", from, from.AddDate(0, 1, -1)); err != nil {
			t.Errorf("month %d: %v", month, err)
		}
	}
}

// A 429 is waited out rather than reported. Without this a rate-limited run
// degenerates: the 429 comes back faster than a real response, so the caller's
// own throttle stops pacing anything and the run accelerates into the wall it
// just hit — which is how one backfill turned 24 consecutive months into
// nothing at all.
func TestGet_WaitsOutARateLimitAndSucceeds(t *testing.T) {
	var calls int
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"Id":1,"PullZoneId":7}`))
	})

	library, err := client.GetVideoLibrary(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetVideoLibrary: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (two refused, one served)", calls)
	}
	if library.PullZoneID != 7 {
		t.Errorf("PullZoneID = %d", library.PullZoneID)
	}
}

// When the limit does not clear, the caller is told with a distinct error, so
// it can stop the whole run instead of failing every remaining area against the
// same wall.
func TestGet_APersistentRateLimitIsItsOwnError(t *testing.T) {
	var calls int
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		// A short Retry-After keeps the test quick while still exercising the
		// header path; the exponential fallback is what runs in production.
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := client.GetVideoLibrary(context.Background(), "1")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if calls != rateLimitRetries+1 {
		t.Errorf("calls = %d, want %d", calls, rateLimitRetries+1)
	}
}

// A backoff must not outlive the run it belongs to: a shutdown should not have
// to sit through a 16-second wait.
func TestGet_BackoffGivesUpWhenTheRunIsCancelled(t *testing.T) {
	client := newAccountClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	start := time.Now()
	if _, err := client.GetVideoLibrary(ctx, "1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waited %v; the backoff ignored cancellation", elapsed)
	}
}

// Retry-After is the server's own answer and beats the caller's guess, but a
// run must not be parked for an hour by one.
func TestRetryAfter_PrefersTheHeaderAndCapsIt(t *testing.T) {
	cases := []struct {
		header   string
		fallback time.Duration
		want     time.Duration
	}{
		{"5", 2 * time.Second, 5 * time.Second},
		{"", 2 * time.Second, 2 * time.Second},
		{"garbage", 4 * time.Second, 4 * time.Second},
		{"0", 8 * time.Second, 8 * time.Second},
		{"9999", 2 * time.Second, maxRetryAfter},
	}
	for _, tc := range cases {
		resp := &http.Response{Header: http.Header{}}
		if tc.header != "" {
			resp.Header.Set("Retry-After", tc.header)
		}
		if got := retryAfter(resp, tc.fallback); got != tc.want {
			t.Errorf("retryAfter(%q, %v) = %v, want %v", tc.header, tc.fallback, got, tc.want)
		}
	}
}
