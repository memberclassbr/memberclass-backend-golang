// Package bunny_usage records how much each area stores on and serves from
// Bunny, one row per area per UTC month in "TenantBunnyMonthlyUsage".
//
// The whole design follows from traffic and storage not being the same kind of
// number. Traffic is a flow accumulated over the month, and Bunny keeps a
// running total the worker can simply overwrite — which is what makes a run
// idempotent and a missed day self-healing. Storage is an instantaneous
// reading with no history behind it anywhere: `GET /statistics` has no storage
// field at all, so a month of storage exists only as the samples this worker
// took during it. That asymmetry is why a lost day of traffic costs nothing
// and a lost day of storage is gone.
//
// It follows too that null is not zero. Every storage column is nullable and
// null means "not measured": a backfilled month has no storage and never will,
// and an area whose library Bunny no longer has (`source = "missing"`) has a
// number nobody knows, which is not the same as an area that used nothing.
package bunny_usage

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

const (
	// lockTTL bounds how long one replica owns a day's run. Every replica in a
	// deployment starts the scheduler, and a run walks every area calling
	// Bunny — two of them at once is the account's rate limit spent twice for
	// one set of numbers, and two samples of the same day racing on the
	// storage average.
	lockTTL = 30 * time.Minute

	lockKeyPrefix = "bunny:usage:sync:"

	// requestThrottle spaces the calls to Bunny.
	//
	// 200ms was measured to be too fast: a full backfill ran clean for about
	// 50 seconds and then took a 429 on every remaining call. The limit is per
	// *account*, not per library or per deployment, so it is shared with the
	// transcription slice's pull-zone lookups and with anything else touching
	// the same key — which is why the safe rate is well under what one run
	// alone appears to get away with.
	//
	// A second is roughly 26 requests a minute once the ~1.3s round trip is
	// counted, comfortably under the wall. It makes a 12-month backfill of ~121
	// areas an hour rather than half of one; that is the right trade, because
	// the backfill runs once and a hole it leaves is a month of history that
	// falls out of Bunny's rolling year before anyone notices.
	requestThrottle = time.Second

	// failureAlertRatio is the share of areas that must fail before a run is
	// reported as failed rather than logged. Below it, a run that lost a few
	// areas is a warning: the numbers are overwritten daily, so one area's bad
	// day costs one day of storage sampling and no traffic at all. Above it,
	// something is wrong with the account or the network rather than with the
	// areas, and it is worth waking someone.
	failureAlertRatio = 0.2

	// closeLookbackMonths bounds how far back the closing pass looks for a
	// month still open. It matches what /statistics answers for: a month older
	// than that cannot be closed from Bunny at all, so scanning for it would
	// only produce a failure every day forever.
	closeLookbackMonths = 12

	// sourceMeasured is a month this worker sampled day by day.
	sourceMeasured = "measured"
	// sourceBackfilled is a month reconstructed from /statistics: traffic
	// only, no storage, ever.
	sourceBackfilled = "backfilled"
	// sourceMissing is an area whose library Bunny answered 404 for. The
	// number is unknown, not zero.
	sourceMissing = "missing"
)

// Store is the slice of Redis this job needs: one key, to keep replicas from
// running the same day twice.
type Store interface {
	SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
}

// Unlocked is a Store that always grants the lock. It is what a deliberate
// manual run from cmd/analytics uses: an operator running the job by hand has
// already decided it should run, and should not be silently skipped because a
// replica took today's lock an hour ago.
type Unlocked struct{}

func (Unlocked) SetNX(context.Context, string, string, time.Duration) (bool, error) { return true, nil }

// Job is an analytics.Job: the scheduler owns when it runs.
type Job struct {
	db    *sql.DB
	bunny bunny.AccountService
	store Store
	log   logger.Logger

	// owner identifies this replica in the lock, so a stuck lock names who
	// holds it.
	owner string

	now      func() time.Time
	throttle time.Duration

	areas    metric.Int64Counter
	failures metric.Int64Counter
	missing  metric.Int64Counter
	closed   metric.Int64Counter
}

func New(db *sql.DB, account bunny.AccountService, store Store, log logger.Logger, owner string) *Job {
	j := &Job{
		db:       db,
		bunny:    account,
		store:    store,
		log:      log,
		owner:    owner,
		now:      time.Now,
		throttle: requestThrottle,
	}

	// The counters are what says the worker ran at all. Storage cannot be
	// reconstructed after the fact, so "it silently stopped three weeks ago" is
	// the failure this job most needs to be caught in — and a job that stops
	// emits nothing, which only shows up against a series that used to.
	meter := otel.GetMeterProvider().Meter("github.com/memberclass-backend-golang/internal/features/workers/bunny_usage")
	j.areas = counter(meter, log, "bunny.usage.areas.synced", "Areas whose Bunny usage was recorded")
	j.failures = counter(meter, log, "bunny.usage.areas.failed", "Areas whose Bunny usage could not be read")
	j.missing = counter(meter, log, "bunny.usage.areas.missing", "Areas whose Bunny library answered 404")
	j.closed = counter(meter, log, "bunny.usage.months.closed", "Months closed from the /statistics period total")

	return j
}

func counter(meter metric.Meter, log logger.Logger, name, description string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(description), metric.WithUnit("{area}"))
	if err != nil {
		// A meter that will not build an instrument is not a reason to stop
		// collecting the usage itself.
		log.Warn("Bunny usage metrics unavailable: " + err.Error())
		return nil
	}
	return c
}

func add(ctx context.Context, c metric.Int64Counter, n int64) {
	if c != nil && n > 0 {
		c.Add(ctx, n)
	}
}

func (j *Job) Name() string { return "bunny_usage.sync" }

// Execute closes any month that has ended, then samples the current one.
//
// Closing first is the order that matters on the 1st: `TrafficUsage` has
// already been reset by Bunny at 00:00 UTC, so the previous month can only be
// read back from `/statistics`, and the sampling pass would otherwise write the
// new month's row while nothing had finished the old one.
func (j *Job) Execute(ctx context.Context) error {
	now := j.now().UTC()
	stamp := now.Format("2006-01-02")

	won, err := j.store.SetNX(ctx, lockKeyPrefix+stamp, j.owner, lockTTL)
	if err != nil {
		return err
	}
	if !won {
		// Another replica has today. Not an error: this is the lock working.
		j.log.Debug("Bunny usage sync skipped: another replica holds " + stamp)
		return nil
	}

	areas, err := j.listAreas(ctx)
	if err != nil {
		return err
	}
	if len(areas) == 0 {
		j.log.Info("Bunny usage sync: no area has a bunnyLibraryId")
		return nil
	}

	closeErr := j.closeFinishedMonths(ctx, now)
	sampleErr := j.sampleCurrentMonth(ctx, areas, now)

	if sampleErr != nil {
		return sampleErr
	}
	return closeErr
}

// area is one tenant row with a Bunny library attached.
type area struct {
	tenantID   string
	libraryID  string
	pullZoneID string
}

// sqlListAreas lists the areas with a library to read.
//
// The tenantId filter every request-path query carries has no equivalent here
// — this job is the one thing in the service that legitimately walks every
// tenant on the deployment.
const sqlListAreas = `
	SELECT id, "bunnyLibraryId", COALESCE("bunnyPullZoneId", '')
	FROM "Tenant"
	WHERE "bunnyLibraryId" IS NOT NULL
	  AND "bunnyLibraryId" <> ''
	ORDER BY id
`

func (j *Job) listAreas(ctx context.Context) ([]area, error) {
	rows, err := j.db.QueryContext(ctx, sqlListAreas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []area
	for rows.Next() {
		var a area
		if err := rows.Scan(&a.tenantID, &a.libraryID, &a.pullZoneID); err != nil {
			return nil, err
		}
		areas = append(areas, a)
	}
	return areas, rows.Err()
}

// pace spaces calls to Bunny, and gives up early when the run is cancelled —
// a shutdown should not wait out the throttle of a hundred remaining areas.
func (j *Job) pace(ctx context.Context) error {
	if j.throttle <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(j.throttle):
		return nil
	}
}
