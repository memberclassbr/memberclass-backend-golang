package apikeyusage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/cache"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// fakeStore records every pipeline the recorder submits and can be made to
// fail.
type fakeStore struct {
	mu    sync.Mutex
	calls [][]cache.HashIncr
	ttls  []time.Duration
	err   error
}

func (s *fakeStore) HIncrByPipeline(_ context.Context, ttl time.Duration, incs ...cache.HashIncr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, incs)
	s.ttls = append(s.ttls, ttl)
	return nil
}

// newTestRecorder pins the clock so the day and the breaker are both
// controllable.
func newTestRecorder(store Store, now func() time.Time) *Recorder {
	r := New(store, fakeLogger{})
	r.now = now
	return r
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestRecord_CountsOneRequestAgainstTheDayHash(t *testing.T) {
	store := &fakeStore{}
	day := time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC)

	newTestRecorder(store, fixedClock(day)).Record(context.Background(), "key-1", "user/informations", 200)

	require.Len(t, store.calls, 1)
	assert.Equal(t, []cache.HashIncr{
		{Key: "apikey:usage:req:2026-08-21", Field: "key-1|user/informations", By: 1},
	}, store.calls[0])
	assert.Equal(t, KeyTTL, store.ttls[0])
}

// errors is a subset of requests, so a failure increments both. That is what
// makes a broken integration a spike in errors rather than in volume.
func TestRecord_AFailureCountsAsRequestAndError(t *testing.T) {
	store := &fakeStore{}
	day := time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC)

	newTestRecorder(store, fixedClock(day)).Record(context.Background(), "key-1", "comments/:commentId", 429)

	require.Len(t, store.calls, 1)
	assert.Equal(t, []cache.HashIncr{
		{Key: "apikey:usage:req:2026-08-21", Field: "key-1|comments/:commentId", By: 1},
		{Key: "apikey:usage:err:2026-08-21", Field: "key-1|comments/:commentId", By: 1},
	}, store.calls[0])
}

// The day comes from UTC, not from the container's local zone: the panel and
// the `date` column are both UTC, and near midnight in Brazil a local clock
// would file the request under the wrong day.
func TestRecord_UsesTheUTCDay(t *testing.T) {
	store := &fakeStore{}
	saoPaulo := time.FixedZone("-03", -3*60*60)
	// 21:30 in São Paulo is already the 22nd in UTC.
	local := time.Date(2026, 8, 21, 21, 30, 0, 0, saoPaulo)

	newTestRecorder(store, fixedClock(local)).Record(context.Background(), "key-1", "user/informations", 200)

	require.Len(t, store.calls, 1)
	assert.Equal(t, "apikey:usage:req:2026-08-22", store.calls[0][0].Key)
}

// A dead Redis must cost the request nothing at all — not an error, and after a
// few attempts not even the timeout.
func TestRecord_BreakerStopsCallingAFailingStore(t *testing.T) {
	store := &fakeStore{err: errors.New("redis down")}
	clock := time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC)
	now := clock
	r := newTestRecorder(store, func() time.Time { return now })

	for i := 0; i < breakerThreshold; i++ {
		r.Record(context.Background(), "key-1", "user/informations", 200)
	}
	assert.False(t, r.allow(), "the breaker should be open after the threshold")

	// Recovered, but still inside the cooldown: nothing is attempted.
	store.err = nil
	r.Record(context.Background(), "key-1", "user/informations", 200)
	assert.Empty(t, store.calls)

	// Past the cooldown it tries again.
	now = clock.Add(breakerCooldown + time.Second)
	r.Record(context.Background(), "key-1", "user/informations", 200)
	assert.Len(t, store.calls, 1)
}

// A cancelled request has still been served, so its count must survive the
// cancellation.
func TestRecord_SurvivesACancelledRequestContext(t *testing.T) {
	store := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	newTestRecorder(store, time.Now).Record(ctx, "key-1", "user/informations", 200)

	assert.Len(t, store.calls, 1)
}

func TestRecord_IgnoresIncompleteInput(t *testing.T) {
	store := &fakeStore{}
	r := newTestRecorder(store, time.Now)

	r.Record(context.Background(), "", "user/informations", 200)
	r.Record(context.Background(), "key-1", "", 200)

	assert.Empty(t, store.calls)
}

// A nil Recorder is what a deployment without Redis has, and it must be
// callable.
func TestRecord_NilRecorderDoesNothing(t *testing.T) {
	var r *Recorder
	assert.NotPanics(t, func() { r.Record(context.Background(), "key-1", "user/informations", 200) })
}

func TestParseField_RoundTrips(t *testing.T) {
	apiKeyID, endpoint, ok := ParseField(Field("key-1", "comments/:commentId"))

	require.True(t, ok)
	assert.Equal(t, "key-1", apiKeyID)
	assert.Equal(t, "comments/:commentId", endpoint)
}

func TestParseField_RejectsAnythingItDidNotWrite(t *testing.T) {
	for _, field := range []string{"", "no-separator", "|endpoint", "key-1|"} {
		_, _, ok := ParseField(field)
		assert.False(t, ok, field)
	}
}

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"/api/v1/user/informations":    "user/informations",
		"/api/v1/comments/{commentId}": "comments/:commentId",
		"/api/v1/vitrine/{vitrineId}":  "vitrine/:vitrineId",
		"/api/v1/lessons/{id:[0-9]+}":  "lessons/:id",
		"/sso/generate-token":          "sso/generate-token",

		// No pattern, a wildcard mount, or a malformed one: no row.
		"":                "",
		"/api/v1/*":       "",
		"/*":              "",
		"/api/v1/":        "",
		"/api/v1/{broken": "",
		"/api/v1/broken}": "",
	}

	for pattern, want := range cases {
		assert.Equal(t, want, Endpoint(pattern), pattern)
	}
}
