// Package api_key_usage folds the per-day Redis counters written by
// internal/platform/apikeyusage into "ApiKeyUsageDaily", and stamps
// "TenantApiKey"."lastUsedAt" from the same pass.
//
// It runs hourly rather than once at midnight. A daily job would leave the
// panel showing nothing for the current day, so an integration that breaks in
// the morning would not appear anywhere until the following night — which is
// most of the value of the errors column. Running it hourly is free because
// the upsert writes the day's total rather than adding to it: the same hour
// flushed twelve times produces the same row.
package api_key_usage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/memberclass-backend-golang/internal/platform/apikeyusage"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

const (
	// scanDays is how far back a run looks. Today plus two, which covers a job
	// that failed for a whole day and stays inside the 72h key TTL.
	scanDays = 3

	// lockTTL bounds how long one replica owns a day. Every replica in a
	// deployment starts the scheduler, and two of them running HGETALL+DEL over
	// the same hash is lost data: whoever deletes first takes the counters out
	// from under the other.
	lockTTL = 15 * time.Minute

	lockKeyPrefix = "apikey:usage:flush:"

	// retentionDays is how long a day's rows are kept. The panel does not read
	// further back, and nothing else does.
	retentionDays = 90

	// pruneHourUTC is when the retention delete runs. Once a day, not once an
	// hour: the table is indexed on (tenantId, date), so a delete filtered on
	// date alone cannot use it and scans.
	pruneHourUTC = 3
)

// Store is the slice of Redis this job needs.
type Store interface {
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
	Delete(ctx context.Context, key string) error
}

// Job is an analytics.Job: the scheduler owns when it runs.
type Job struct {
	db    *sql.DB
	store Store
	log   logger.Logger

	// owner identifies this replica in the lock, so a stuck lock names who
	// holds it.
	owner string

	now func() time.Time
}

func New(db *sql.DB, store Store, log logger.Logger, owner string) *Job {
	return &Job{db: db, store: store, log: log, owner: owner, now: time.Now}
}

func (j *Job) Name() string { return "api_key_usage.flush" }

// Execute flushes every day still present in Redis, newest first.
//
// A day that has ended is deleted from Redis once written; the current day is
// left in place, because it is still being counted. That asymmetry is the only
// reason the job needs to know what day it is.
func (j *Job) Execute(ctx context.Context) error {
	today := j.now().UTC().Truncate(24 * time.Hour)

	var firstErr error
	for i := 0; i < scanDays; i++ {
		day := today.AddDate(0, 0, -i)
		final := i > 0

		if err := j.flushDay(ctx, day, final); err != nil {
			j.log.Error("API key usage flush failed for " + day.Format(apikeyusage.DayFormat) + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if j.now().UTC().Hour() == pruneHourUTC {
		if err := j.prune(ctx); err != nil {
			j.log.Error("API key usage prune failed: " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// flushDay writes one day's counters. final says the day has ended, which is
// what licenses deleting the hashes afterwards.
func (j *Job) flushDay(ctx context.Context, day time.Time, final bool) error {
	stamp := day.Format(apikeyusage.DayFormat)

	won, err := j.store.SetNX(ctx, lockKeyPrefix+stamp, j.owner, lockTTL)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !won {
		// Another replica has this day. Not an error: this is the lock working.
		j.log.Debug("API key usage flush skipped for " + stamp + ": another replica holds the lock")
		return nil
	}

	requests, err := j.store.HGetAll(ctx, apikeyusage.RequestsKey(day))
	if err != nil {
		return fmt.Errorf("read request counters: %w", err)
	}
	errorsByField, err := j.store.HGetAll(ctx, apikeyusage.ErrorsKey(day))
	if err != nil {
		return fmt.Errorf("read error counters: %w", err)
	}

	// An ended day with nothing in it is the common case — the job runs every
	// hour and only one run per day closes one out — so it is left alone
	// rather than deleted, which would be two Redis calls an hour to remove
	// keys that do not exist.
	empty := len(requests) == 0 && len(errorsByField) == 0

	rows := j.rowsFrom(requests, errorsByField)
	if len(rows) == 0 {
		if final && !empty {
			j.deleteDay(ctx, day)
		}
		return nil
	}

	// A key deleted in the panel takes its usage rows with it, so a row is
	// only written for a key that still exists. Resolving the tenant here also
	// keeps it out of the Redis field, where it would be a third component
	// that can go stale.
	tenants, err := j.tenantsFor(ctx, rows)
	if err != nil {
		return fmt.Errorf("resolve tenants: %w", err)
	}

	live := rows[:0]
	for _, row := range rows {
		if tenantID, ok := tenants[row.apiKeyID]; ok {
			row.tenantID = tenantID
			live = append(live, row)
		}
	}
	if len(live) == 0 {
		if final {
			j.deleteDay(ctx, day)
		}
		return nil
	}

	if err := j.upsert(ctx, day, live); err != nil {
		return fmt.Errorf("upsert usage: %w", err)
	}
	if err := j.stampLastUsed(ctx, day, live); err != nil {
		return fmt.Errorf("stamp lastUsedAt: %w", err)
	}

	if final {
		j.deleteDay(ctx, day)
	}
	return nil
}

// usageRow is one line of "ApiKeyUsageDaily" in flight.
type usageRow struct {
	apiKeyID string
	tenantID string
	endpoint string
	requests int64
	errors   int64
}

// rowsFrom pairs the two hashes by field. A field present only in the errors
// hash is dropped: an error is a subset of a request, so a field without a
// request count is corruption, not data.
func (j *Job) rowsFrom(requests, errorCounts map[string]string) []*usageRow {
	rows := make([]*usageRow, 0, len(requests))

	for field, raw := range requests {
		apiKeyID, endpoint, ok := apikeyusage.ParseField(field)
		if !ok {
			continue
		}
		count, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || count <= 0 {
			continue
		}

		row := &usageRow{apiKeyID: apiKeyID, endpoint: endpoint, requests: count}
		if rawErrors, found := errorCounts[field]; found {
			if errCount, convErr := strconv.ParseInt(rawErrors, 10, 64); convErr == nil && errCount > 0 {
				row.errors = errCount
			}
		}
		rows = append(rows, row)
	}

	return rows
}

const sqlTenantsForKeys = `
	SELECT id, "tenantId"
	FROM "TenantApiKey"
	WHERE id = ANY($1)
`

func (j *Job) tenantsFor(ctx context.Context, rows []*usageRow) (map[string]string, error) {
	seen := make(map[string]struct{}, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.apiKeyID]; ok {
			continue
		}
		seen[row.apiKeyID] = struct{}{}
		ids = append(ids, row.apiKeyID)
	}

	result, err := j.db.QueryContext(ctx, sqlTenantsForKeys, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer result.Close()

	tenants := make(map[string]string, len(ids))
	for result.Next() {
		var id, tenantID string
		if err := result.Scan(&id, &tenantID); err != nil {
			return nil, err
		}
		tenants[id] = tenantID
	}
	return tenants, result.Err()
}

// upsert writes the day's totals. The counters in Redis are cumulative for the
// whole day, so the update assigns rather than adds — that is what makes the
// hourly re-run, and any retry, idempotent.
func (j *Job) upsert(ctx context.Context, day time.Time, rows []*usageRow) error {
	var (
		values = make([]string, 0, len(rows))
		args   = make([]any, 0, len(rows)*5+1)
	)

	args = append(args, day)
	for i, row := range rows {
		base := i*5 + 2
		values = append(values, fmt.Sprintf(
			"(gen_random_uuid(), $%d, $%d, $1, $%d, $%d, $%d)",
			base, base+1, base+2, base+3, base+4,
		))
		args = append(args, row.tenantID, row.apiKeyID, row.endpoint, row.requests, row.errors)
	}

	// gen_random_uuid() supplies the primary key. Prisma's @default(cuid()) is
	// generated by the client, not by the database, so an insert from here has
	// to bring its own id; nothing reads it, since the row's identity is
	// (apiKeyId, date, endpoint).
	query := `
		INSERT INTO "ApiKeyUsageDaily" (id, "tenantId", "apiKeyId", date, endpoint, requests, errors)
		VALUES ` + strings.Join(values, ", ") + `
		ON CONFLICT ("apiKeyId", date, endpoint) DO UPDATE
		SET requests = EXCLUDED.requests,
		    errors   = EXCLUDED.errors`

	_, err := j.db.ExecContext(ctx, query, args...)
	return err
}

// sqlStampLastUsed only moves the column forward. Days are flushed newest
// first, so without the guard the two-day-old scan would drag a key's
// lastUsedAt backwards over the value today's flush just wrote.
const sqlStampLastUsed = `
	UPDATE "TenantApiKey"
	SET "lastUsedAt" = $2
	WHERE id = ANY($1)
	  AND ("lastUsedAt" IS NULL OR "lastUsedAt" < $2)
`

func (j *Job) stampLastUsed(ctx context.Context, day time.Time, rows []*usageRow) error {
	seen := make(map[string]struct{}, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.apiKeyID]; ok {
			continue
		}
		seen[row.apiKeyID] = struct{}{}
		ids = append(ids, row.apiKeyID)
	}

	// The day, not the moment. The panel renders a date and nothing else, and
	// midnight UTC of the day the request happened is the only value the hourly
	// re-run does not keep rewriting.
	_, err := j.db.ExecContext(ctx, sqlStampLastUsed, pq.Array(ids), day)
	return err
}

func (j *Job) deleteDay(ctx context.Context, day time.Time) {
	for _, key := range []string{apikeyusage.RequestsKey(day), apikeyusage.ErrorsKey(day)} {
		if err := j.store.Delete(ctx, key); err != nil {
			// The keys carry a TTL, so a failed delete costs storage for a few
			// days and nothing else. The rows are already written, and writing
			// them again is a no-op.
			j.log.Warn("API key usage counters not cleared for " + key + ": " + err.Error())
		}
	}
}

const sqlPrune = `
	DELETE FROM "ApiKeyUsageDaily"
	WHERE date < $1
`

func (j *Job) prune(ctx context.Context) error {
	cutoff := j.now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)
	result, err := j.db.ExecContext(ctx, sqlPrune, cutoff)
	if err != nil {
		return err
	}
	if deleted, err := result.RowsAffected(); err == nil && deleted > 0 {
		j.log.Info("Pruned " + strconv.FormatInt(deleted, 10) + " API key usage rows older than " + cutoff.Format(apikeyusage.DayFormat))
	}
	return nil
}
