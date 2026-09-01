package bunny_usage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

// MaxBackfillMonths is how far back a backfill can reach. It is Bunny's
// retention and not a choice: `/statistics` refuses a dateFrom older than a
// year, so a thirteenth month has no source.
const MaxBackfillMonths = 12

// Backfill reconstructs past months from `/statistics`, one call per pull zone
// per month.
//
// It is worth running as early as possible and exactly once. Bunny's window is
// a *rolling* year, so every month of delay erases the oldest month from the
// far end permanently — the same reason the daily worker is worth switching on
// before the panel that reads it is finished.
//
// A backfilled row carries traffic and nothing else. `/statistics` has no
// storage field, so every storage column and `lastSampleDate` stay null, which
// is the row saying "not measured" rather than "measured zero". Months older
// than the retention are left with no row at all, for the same reason: an
// absent row is "we don't know", and a zeroed one is a claim.
//
// It never overwrites. A month this worker already measured, or already
// closed, is better than anything a reconstruction can offer, so an existing
// row is skipped before the request is spent rather than after.
func (j *Job) Backfill(ctx context.Context, months int, tenantID string) error {
	if months <= 0 || months > MaxBackfillMonths {
		months = MaxBackfillMonths
	}

	areas, err := j.listAreas(ctx)
	if err != nil {
		return err
	}

	now := j.now().UTC()
	var written, skipped, failed int

	for _, a := range areas {
		if tenantID != "" && a.tenantID != tenantID {
			continue
		}

		pullZoneID, err := j.resolvePullZone(ctx, a)
		if err != nil {
			if errors.Is(err, bunny.ErrUnauthorized) || errors.Is(err, bunny.ErrRateLimited) {
				return fmt.Errorf("bunny backfill aborted after %d rows: %w", written, err)
			}
			failed++
			j.log.Error("Bunny backfill skipped area",
				"tenantId", a.tenantID, "bunnyLibraryId", a.libraryID, "err", err.Error())
			continue
		}

		have, err := j.existingMonths(ctx, a.tenantID)
		if err != nil {
			return err
		}

		for back := 1; back <= months; back++ {
			target := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -back, 0)
			year, month := target.Year(), int(target.Month())

			if _, ok := have[year*12+month]; ok {
				skipped++
				continue
			}

			if err := j.pace(ctx); err != nil {
				return err
			}

			from, to := monthBounds(year, month)
			stats, err := j.bunny.GetStatistics(ctx, pullZoneID, from, to)
			switch {
			case errors.Is(err, bunny.ErrUnauthorized), errors.Is(err, bunny.ErrRateLimited):
				// Stopping here is what keeps a rate-limited run from becoming
				// a hole in the data. The backfill is resumable by design —
				// an existing row is skipped before its request is spent — so
				// running it again after the limit clears picks up exactly
				// where this left off.
				return fmt.Errorf("bunny backfill aborted after %d rows: %w", written, err)

			case errors.Is(err, bunny.ErrStatisticsOutOfRange):
				// Past the retention. Everything older is too, so this area is
				// done rather than worth eleven more refusals.
				j.log.Debug(fmt.Sprintf("Bunny backfill reached the retention limit for tenant %s at %04d-%02d",
					a.tenantID, year, month))
				back = months

			case err != nil:
				failed++
				j.log.Error("Bunny backfill month failed",
					"tenantId", a.tenantID, "month", fmt.Sprintf("%04d-%02d", year, month), "err", err.Error())

			default:
				if err := j.insertBackfilled(ctx, a, year, month, stats.TotalBandwidthUsed, pullZoneID); err != nil {
					failed++
					j.log.Error("Bunny backfill row not written",
						"tenantId", a.tenantID, "month", fmt.Sprintf("%04d-%02d", year, month), "err", err.Error())
					continue
				}
				written++
			}
		}
	}

	j.log.Info(fmt.Sprintf("Bunny backfill: %d months written, %d already present, %d failed",
		written, skipped, failed))
	return nil
}

// resolvePullZone reuses `Tenant.bunnyPullZoneId` when it is set and asks Bunny
// once when it is not, storing what it learns. A pull zone belongs to a library
// for the library's life, so this is a lookup an area pays for exactly once.
func (j *Job) resolvePullZone(ctx context.Context, a area) (string, error) {
	if a.pullZoneID != "" {
		return a.pullZoneID, nil
	}

	library, err := j.bunny.GetVideoLibrary(ctx, a.libraryID)
	if err != nil {
		return "", err
	}
	if library.PullZoneID == 0 {
		return "", fmt.Errorf("library %s has no PullZoneId", a.libraryID)
	}

	pullZoneID := strconv.Itoa(library.PullZoneID)
	if _, err := j.db.ExecContext(ctx, sqlStorePullZone, a.tenantID, pullZoneID); err != nil {
		// Not fatal: the backfill can run on the value it just read and the
		// next daily pass will store it.
		j.log.Warn("Bunny pull zone not stored", "tenantId", a.tenantID, "err", err.Error())
	}
	return pullZoneID, nil
}

const sqlStorePullZone = `
	UPDATE "Tenant"
	SET "bunnyPullZoneId" = $2
	WHERE id = $1 AND COALESCE("bunnyPullZoneId", '') = ''
`

const sqlExistingMonths = `
	SELECT year, month
	FROM "TenantBunnyMonthlyUsage"
	WHERE "tenantId" = $1
`

func (j *Job) existingMonths(ctx context.Context, tenantID string) (map[int]struct{}, error) {
	rows, err := j.db.QueryContext(ctx, sqlExistingMonths, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	have := map[int]struct{}{}
	for rows.Next() {
		var year, month int
		if err := rows.Scan(&year, &month); err != nil {
			return nil, err
		}
		have[year*12+month] = struct{}{}
	}
	return have, rows.Err()
}

// sqlInsertBackfilled names only the columns a reconstruction can fill. Every
// storage column and lastSampleDate are left out rather than set, so they stay
// null — the difference between "Bunny has no storage history" and "this area
// stored nothing".
//
// closedAt is stamped on the way in: a past month is already final, and the
// daily pass must not treat a reconstructed row as one it can still sample.
const sqlInsertBackfilled = `
	INSERT INTO "TenantBunnyMonthlyUsage" (
		"tenantId", year, month, "trafficBytes", source,
		"bunnyLibraryId", "bunnyPullZoneId", "closedAt", "syncedAt"
	)
	VALUES ($1, $2, $3, $4, '` + sourceBackfilled + `', $5, $6, now(), now())
	ON CONFLICT ("tenantId", year, month) DO NOTHING
`

func (j *Job) insertBackfilled(ctx context.Context, a area, year, month int, trafficBytes int64, pullZoneID string) error {
	_, err := j.db.ExecContext(ctx, sqlInsertBackfilled,
		a.tenantID, year, month, trafficBytes, a.libraryID, pullZoneID)
	return err
}
