// Package apikeyusage counts requests per API key, day and endpoint in Redis.
//
// It is deliberately not a database write. The counters live in two hashes per
// UTC day and a scheduled job folds them into "ApiKeyUsageDaily" — see
// internal/features/workers/api_key_usage. Nothing here is on the durability
// path: what it feeds is a usage panel, and a request must never be slower, or
// fail, because a counter could not be written.
//
// The shape of the Redis keys is the contract between this package and that
// job, so both sides use the helpers below rather than formatting strings.
package apikeyusage

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/memberclass-backend-golang/internal/platform/cache"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

const (
	// RequestsKeyPrefix and ErrorsKeyPrefix each name one hash per UTC day.
	// Two hashes rather than one with suffixed fields: the job reads both with
	// two HGETALLs and never has to parse a counter kind out of a field name.
	RequestsKeyPrefix = "apikey:usage:req:"
	ErrorsKeyPrefix   = "apikey:usage:err:"

	// DayFormat is the day component of a key. It is also what the job parses
	// back to build the `date` column.
	DayFormat = "2006-01-02"

	// KeyTTL outlives the job's three-day scan window (see the worker), so a
	// job that fails for a whole day still finds the counters on its next run.
	// Without it a hash whose job never succeeds would live forever.
	KeyTTL = 72 * time.Hour

	// fieldSeparator joins the two halves of a hash field. Neither half can
	// contain it: an id is a cuid and an endpoint is a chi route pattern.
	fieldSeparator = "|"
)

const (
	// recordTimeout bounds the one Redis round trip a request pays for. It is
	// short on purpose — a Redis this slow has already stopped being useful,
	// and the request is waiting.
	recordTimeout = 100 * time.Millisecond

	// breakerThreshold and breakerCooldown stop a dead Redis from charging
	// every request the full timeout. Without them "degrade quietly" means
	// adding 100ms to every request in the service.
	breakerThreshold = 5
	breakerCooldown  = 30 * time.Second
)

// Store is the slice of Redis this package needs. *cache.RedisCache satisfies
// it; a test satisfies it with a struct that records the increments.
type Store interface {
	HIncrByPipeline(ctx context.Context, ttl time.Duration, incs ...cache.HashIncr) error
}

// Recorder increments the per-day counters. The zero value is not usable; call
// New. A nil *Recorder is usable and does nothing, which is what a deployment
// without Redis gets.
type Recorder struct {
	store Store
	log   logger.Logger

	// now is time.Now in production and a stub in tests, which is the only way
	// to exercise the breaker without sleeping.
	now func() time.Time

	mu        sync.Mutex
	failures  int
	openUntil time.Time

	// errors counts failed writes. Degrading quietly is right for the request
	// and wrong for the operator: without this the panel shows "Never" for
	// every key and nothing anywhere says why.
	errors metric.Int64Counter
}

func New(store Store, log logger.Logger) *Recorder {
	r := &Recorder{store: store, log: log, now: time.Now}

	meter := otel.GetMeterProvider().Meter("github.com/memberclass-backend-golang/internal/platform/apikeyusage")
	counter, err := meter.Int64Counter(
		"apikey.usage.record.errors",
		metric.WithDescription("API key usage counter writes that failed"),
		metric.WithUnit("{write}"),
	)
	if err != nil {
		// A meter that will not build an instrument is not a reason to run
		// without usage counting.
		log.Warn("API key usage metrics unavailable: " + err.Error())
	} else {
		r.errors = counter
	}

	return r
}

// Record counts one finished request against apiKeyID. A status of 400 or more
// counts as both a request and an error: the two columns exist so a broken
// integration shows up as a spike in errors rather than in volume, and a 429 is
// exactly that signal.
//
// It never returns an error and never blocks on the caller's context: a client
// that hangs up mid-request has still made the request.
func (r *Recorder) Record(ctx context.Context, apiKeyID, endpoint string, status int) {
	if r == nil || r.store == nil || apiKeyID == "" || endpoint == "" {
		return
	}
	if !r.allow() {
		return
	}

	day := r.now().UTC().Format(DayFormat)
	field := Field(apiKeyID, endpoint)

	incs := []cache.HashIncr{{Key: RequestsKeyPrefix + day, Field: field, By: 1}}
	if status >= 400 {
		incs = append(incs, cache.HashIncr{Key: ErrorsKeyPrefix + day, Field: field, By: 1})
	}

	// WithoutCancel keeps the write alive past the request it describes, but
	// under a deadline of its own rather than none.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()

	if err := r.store.HIncrByPipeline(writeCtx, KeyTTL, incs...); err != nil {
		r.fail(err)
		return
	}
	r.succeed()
}

// allow reports whether the breaker is closed, i.e. whether Redis is worth
// trying right now.
func (r *Recorder) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.now().Before(r.openUntil)
}

func (r *Recorder) fail(err error) {
	if r.errors != nil {
		r.errors.Add(context.Background(), 1)
	}

	r.mu.Lock()
	r.failures++
	tripped := r.failures >= breakerThreshold
	if tripped {
		r.failures = 0
		r.openUntil = r.now().Add(breakerCooldown)
	}
	r.mu.Unlock()

	// Only the trip is logged. Logging every failure would put a line on every
	// request for as long as Redis is down, which is the same outage reported
	// once per request.
	if tripped {
		r.log.Warn("API key usage counters paused for " + breakerCooldown.String() + ": " + err.Error())
	}
}

func (r *Recorder) succeed() {
	r.mu.Lock()
	r.failures = 0
	r.mu.Unlock()
}

// Field builds the hash field the counters are stored under.
func Field(apiKeyID, endpoint string) string {
	return apiKeyID + fieldSeparator + endpoint
}

// ParseField splits a hash field back into its two halves. It reports false for
// anything it did not write, so a stray field cannot become a row.
func ParseField(field string) (apiKeyID, endpoint string, ok bool) {
	apiKeyID, endpoint, ok = strings.Cut(field, fieldSeparator)
	if !ok || apiKeyID == "" || endpoint == "" {
		return "", "", false
	}
	return apiKeyID, endpoint, true
}

// RequestsKey and ErrorsKey name the two hashes for a UTC day.
func RequestsKey(day time.Time) string { return RequestsKeyPrefix + day.UTC().Format(DayFormat) }
func ErrorsKey(day time.Time) string   { return ErrorsKeyPrefix + day.UTC().Format(DayFormat) }
