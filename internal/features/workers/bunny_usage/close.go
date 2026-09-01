package bunny_usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

// closeFinishedMonths finalises every month that has ended and is still open.
//
// It cannot use the library's own counter. `TrafficUsage` is documented as the
// traffic *this month*, and Bunny zeroes it at the UTC turn of the 1st — so a
// run at 06:00 UTC on the 1st (03:00 in Brasília) would read a counter that was
// reset six hours earlier and lose the last ~21 hours of every month, in a
// number that is passed on to the customer. `/statistics` over the closed
// period is immune to the reset, which is why the time of day this runs is
// irrelevant for the current month and decisive for a finished one.
//
// A month is closed once. After `closedAt` the daily pass no longer touches the
// row, so the figure a customer was billed from stays the figure that was read.
func (j *Job) closeFinishedMonths(ctx context.Context, now time.Time) error {
	months, err := j.listOpenFinishedMonths(ctx, now)
	if err != nil {
		return err
	}
	if len(months) == 0 {
		return nil
	}

	var closed, failed int

	for i, m := range months {
		if i > 0 {
			if err := j.pace(ctx); err != nil {
				return err
			}
		}

		if m.pullZoneID == "" {
			// No pull zone means no way to reach /statistics. It resolves
			// itself the moment a daily pass reads the library, so the month
			// stays open and is retried; a library that is gone for good falls
			// out of the lookback window rather than being closed from a
			// number nobody has.
			j.log.Warn("Bunny month cannot be closed: no pull zone",
				"tenantId", m.tenantID, "month", fmt.Sprintf("%04d-%02d", m.year, m.month))
			continue
		}

		err := j.closeMonth(ctx, m)
		switch {
		case errors.Is(err, bunny.ErrUnauthorized), errors.Is(err, bunny.ErrRateLimited):
			// Systemic, so the run stops rather than failing every remaining
			// month against the same wall. Nothing is lost by stopping: an
			// unclosed month keeps its closedAt null and is picked up again
			// tomorrow.
			add(ctx, j.closed, int64(closed))
			return fmt.Errorf("bunny month close aborted after %d/%d months: %w", i, len(months), err)

		case errors.Is(err, bunny.ErrStatisticsOutOfRange):
			// Older than Bunny answers for. Nothing here will ever close it,
			// and it leaves the scan on its own as the lookback window moves.
			j.log.Warn("Bunny month is older than the statistics retention and stays open",
				"tenantId", m.tenantID, "month", fmt.Sprintf("%04d-%02d", m.year, m.month))

		case err != nil:
			failed++
			j.log.Error("Bunny month close failed",
				"tenantId", m.tenantID, "month", fmt.Sprintf("%04d-%02d", m.year, m.month), "err", err.Error())

		default:
			closed++
		}
	}

	add(ctx, j.closed, int64(closed))

	if closed > 0 || failed > 0 {
		j.log.Info(fmt.Sprintf("Bunny month close: %d closed, %d failed of %d open months",
			closed, failed, len(months)))
	}

	// The same threshold as the sampling pass, for the same reason: a few
	// months failing is retried tomorrow at no cost, and a majority failing is
	// not about the months.
	if ratio := float64(failed) / float64(len(months)); ratio > failureAlertRatio {
		return fmt.Errorf("bunny month close: %d of %d months failed (%.0f%%)", failed, len(months), ratio*100)
	}
	return nil
}

// openMonth is one row waiting to be closed.
type openMonth struct {
	tenantID   string
	year       int
	month      int
	pullZoneID string
}

// sqlOpenFinishedMonths finds the rows of months that have ended and carry no
// closedAt.
//
// Ordering by (year, month) as an integer is what `year * 12 + month` is for:
// it makes "before the current month" one comparison rather than a pair of
// them, and it is the same arithmetic the lookback bound uses.
//
// The pull zone is read from the usage row first and the tenant second: the row
// is a snapshot of where that month's numbers came from, and it stays right for
// an area whose library has since been replaced.
const sqlOpenFinishedMonths = `
	SELECT u."tenantId", u.year, u.month,
	       COALESCE(NULLIF(u."bunnyPullZoneId", ''), COALESCE(t."bunnyPullZoneId", ''))
	FROM "TenantBunnyMonthlyUsage" u
	JOIN "Tenant" t ON t.id = u."tenantId"
	WHERE u."closedAt" IS NULL
	  AND (u.year * 12 + u.month) < $1
	  AND (u.year * 12 + u.month) >= $2
	ORDER BY u.year, u.month, u."tenantId"
`

func (j *Job) listOpenFinishedMonths(ctx context.Context, now time.Time) ([]openMonth, error) {
	current := now.Year()*12 + int(now.Month())

	rows, err := j.db.QueryContext(ctx, sqlOpenFinishedMonths, current, current-closeLookbackMonths)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []openMonth
	for rows.Next() {
		var m openMonth
		if err := rows.Scan(&m.tenantID, &m.year, &m.month, &m.pullZoneID); err != nil {
			return nil, err
		}
		months = append(months, m)
	}
	return months, rows.Err()
}

func (j *Job) closeMonth(ctx context.Context, m openMonth) error {
	from, to := monthBounds(m.year, m.month)

	stats, err := j.bunny.GetStatistics(ctx, m.pullZoneID, from, to)
	if err != nil {
		return err
	}

	// TotalBandwidthUsed already sums the period; BandwidthUsedChart is the
	// same figure spread over days and does not need adding up.
	_, err = j.db.ExecContext(ctx, sqlCloseMonth, m.tenantID, m.year, m.month, stats.TotalBandwidthUsed, m.pullZoneID)
	return err
}

// sqlCloseMonth writes the period total and stamps closedAt.
//
// The `closedAt IS NULL` predicate makes the close itself idempotent: two
// replicas that somehow both reach this month write it once, and the second
// finds nothing to update rather than re-stamping a different timestamp on a
// row someone has already read a bill off.
//
// Storage is untouched. There is no storage in a /statistics response, so a
// close has nothing to say about it and must not overwrite the samples the
// month collected.
const sqlCloseMonth = `
	UPDATE "TenantBunnyMonthlyUsage"
	SET "trafficBytes"    = $4,
	    "bunnyPullZoneId" = COALESCE("bunnyPullZoneId", NULLIF($5::text, '')),
	    "closedAt"        = now(),
	    "syncedAt"        = now()
	WHERE "tenantId" = $1 AND year = $2 AND month = $3 AND "closedAt" IS NULL
`

// ReopenAndCloseMonth reprocesses one month that is already closed.
//
// It is the deliberate manual operation the closing rule leaves room for: the
// daily pass will not touch a closed row, on purpose, so correcting one is
// something an operator asks for by name through cmd/analytics rather than
// something a schedule can decide to do.
func (j *Job) ReopenAndCloseMonth(ctx context.Context, tenantID string, year, month int) error {
	var pullZoneID string
	err := j.db.QueryRowContext(ctx, sqlPullZoneForMonth, tenantID, year, month).Scan(&pullZoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no usage row for tenant %s in %04d-%02d", tenantID, year, month)
	}
	if err != nil {
		return err
	}
	if pullZoneID == "" {
		return fmt.Errorf("tenant %s has no bunnyPullZoneId; run the daily sync first", tenantID)
	}

	from, to := monthBounds(year, month)
	stats, err := j.bunny.GetStatistics(ctx, pullZoneID, from, to)
	if err != nil {
		return err
	}

	_, err = j.db.ExecContext(ctx, sqlReopenAndCloseMonth, tenantID, year, month, stats.TotalBandwidthUsed)
	if err != nil {
		return err
	}

	j.log.Info(fmt.Sprintf("Bunny month %04d-%02d reprocessed for tenant %s: trafficBytes=%d",
		year, month, tenantID, stats.TotalBandwidthUsed))
	return nil
}

const sqlPullZoneForMonth = `
	SELECT COALESCE(NULLIF(u."bunnyPullZoneId", ''), COALESCE(t."bunnyPullZoneId", ''))
	FROM "TenantBunnyMonthlyUsage" u
	JOIN "Tenant" t ON t.id = u."tenantId"
	WHERE u."tenantId" = $1 AND u.year = $2 AND u.month = $3
`

// sqlReopenAndCloseMonth carries no closedAt predicate: overwriting a closed
// month is the entire point of the call, and the new closedAt records when the
// correction was made.
const sqlReopenAndCloseMonth = `
	UPDATE "TenantBunnyMonthlyUsage"
	SET "trafficBytes" = $4,
	    "closedAt"     = now(),
	    "syncedAt"     = now()
	WHERE "tenantId" = $1 AND year = $2 AND month = $3
`

// monthBounds returns the first and last UTC day of a month, both inclusive —
// which is how /statistics reads dateFrom and dateTo.
func monthBounds(year, month int) (from, to time.Time) {
	from = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 1, -1)
	return from, to
}
