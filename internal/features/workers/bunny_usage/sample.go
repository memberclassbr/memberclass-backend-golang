package bunny_usage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

// sampleCurrentMonth reads every area's library once and writes the current UTC
// month's row.
//
// A per-area failure is not a failed run: traffic is overwritten from a running
// total, so tomorrow's pass repairs it, and one missed storage sample moves the
// month's average by one reading. What is worth reporting is a systemic
// failure — a rejected account key, or so many areas failing at once that the
// cause cannot be the areas.
func (j *Job) sampleCurrentMonth(ctx context.Context, areas []area, now time.Time) error {
	year, month := now.Year(), int(now.Month())
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var synced, failed, missing int

	for i, a := range areas {
		if i > 0 {
			if err := j.pace(ctx); err != nil {
				return err
			}
		}

		library, err := j.bunny.GetVideoLibrary(ctx, a.libraryID)
		switch {
		case errors.Is(err, bunny.ErrUnauthorized), errors.Is(err, bunny.ErrRateLimited):
			// Both are systemic: one rejected key fails every area, and an
			// exhausted rate limit means the next area meets the same wall.
			// Carrying on would spend a request per area to learn the same
			// thing and bury the cause under a hundred identical failures —
			// and in the rate-limited case it makes the wall worse, since a
			// 429 comes back faster than a real response and the run
			// accelerates into it.
			add(ctx, j.areas, int64(synced))
			add(ctx, j.failures, int64(failed))
			add(ctx, j.missing, int64(missing))
			return fmt.Errorf("bunny usage sync aborted after %d/%d areas: %w", i, len(areas), err)

		case errors.Is(err, bunny.ErrLibraryNotFound):
			// An orphaned library is expected — an area can be deleted on
			// Bunny's side and keep its column here. It is recorded as unknown
			// usage and does not count as a failure, or the daily noise would
			// train everyone to ignore the alert.
			missing++
			j.log.Warn("Bunny library not found",
				"tenantId", a.tenantID, "bunnyLibraryId", a.libraryID)
			if err := j.upsertMissing(ctx, a, year, month); err != nil {
				failed++
				j.log.Error("Bunny usage row not written for missing library",
					"tenantId", a.tenantID, "err", err.Error())
			}
			continue

		case err != nil:
			failed++
			j.log.Error("Bunny library read failed",
				"tenantId", a.tenantID, "bunnyLibraryId", a.libraryID, "err", err.Error())
			continue
		}

		pullZoneID := ""
		if library.PullZoneID != 0 {
			pullZoneID = strconv.Itoa(library.PullZoneID)
		}

		if err := j.upsertSample(ctx, a, year, month, day, library, pullZoneID); err != nil {
			failed++
			j.log.Error("Bunny usage row not written",
				"tenantId", a.tenantID, "err", err.Error())
			continue
		}

		// The three Tenant columns are what the manager reads today. They go
		// when the tenant usage screen exists; until then, not writing them
		// means the screen quietly stops moving.
		if err := j.mirrorToTenant(ctx, a.tenantID, library, pullZoneID); err != nil {
			// The month row is already written, which is the durable half.
			// Failing the area here would double-count it as a failure for a
			// mirror nothing computes from.
			j.log.Warn("Bunny usage mirror to Tenant failed",
				"tenantId", a.tenantID, "err", err.Error())
		}

		synced++
	}

	add(ctx, j.areas, int64(synced))
	add(ctx, j.failures, int64(failed))
	add(ctx, j.missing, int64(missing))

	j.log.Info(fmt.Sprintf("Bunny usage sync: %d synced, %d missing, %d failed of %d areas",
		synced, missing, failed, len(areas)))

	if ratio := float64(failed) / float64(len(areas)); ratio > failureAlertRatio {
		return fmt.Errorf("bunny usage sync: %d of %d areas failed (%.0f%%)", failed, len(areas), ratio*100)
	}
	return nil
}

// sqlUpsertSample writes one area's current month.
//
// Traffic is assigned, not added: `TrafficUsage` is Bunny's own running total
// for the month, so writing it twice in a day writes the same number twice.
// Storage is accumulated, and `lastSampleDate` is the guard that makes it safe
// to run more than once a day — a second run on the same UTC day refreshes the
// last reading and the traffic without touching the sum or the count, which
// would otherwise weight one day double in the average.
//
// The `closedAt IS NULL` guard is the whole of the rewrite lock on a closed
// month: a closed row is a billing record read from a finished period, and the
// daily pass has nothing better to say about it.
const sqlUpsertSample = `
	INSERT INTO "TenantBunnyMonthlyUsage" AS u (
		"tenantId", year, month,
		"trafficBytes",
		"storageLastBytes", "storageSampleSum", "storageSampleCount", "storageAvgBytes",
		"lastSampleDate", source, "bunnyLibraryId", "bunnyPullZoneId", "syncedAt"
	)
	VALUES ($1, $2, $3, $4, $5, $5, 1, $5, $6, '` + sourceMeasured + `', $7, NULLIF($8::text, ''), now())
	ON CONFLICT ("tenantId", year, month) DO UPDATE SET
		"trafficBytes"       = EXCLUDED."trafficBytes",
		"storageLastBytes"   = EXCLUDED."storageLastBytes",
		"storageSampleSum"   = ` + sqlNewSum + `,
		"storageSampleCount" = ` + sqlNewCount + `,
		"storageAvgBytes"    = ` + sqlNewSum + ` / NULLIF(` + sqlNewCount + `, 0),
		"lastSampleDate"     = EXCLUDED."lastSampleDate",
		source               = '` + sourceMeasured + `',
		"bunnyLibraryId"     = EXCLUDED."bunnyLibraryId",
		"bunnyPullZoneId"    = COALESCE(EXCLUDED."bunnyPullZoneId", u."bunnyPullZoneId"),
		"syncedAt"           = now()
	WHERE u."closedAt" IS NULL
`

// sqlNewSum and sqlNewCount are spelled out three times in the upsert because
// the average has to be computed from the values the same statement is
// assigning, and an ON CONFLICT DO UPDATE cannot read a column it is writing.
//
// IS DISTINCT FROM rather than <>: `lastSampleDate` is null on a row a
// "missing" day created, and a null comparison would make the day look already
// sampled and drop the reading.
const (
	sqlNewSum = `CASE WHEN u."lastSampleDate" IS DISTINCT FROM EXCLUDED."lastSampleDate"
			THEN COALESCE(u."storageSampleSum", 0) + EXCLUDED."storageLastBytes"
			ELSE COALESCE(u."storageSampleSum", EXCLUDED."storageLastBytes") END`

	sqlNewCount = `CASE WHEN u."lastSampleDate" IS DISTINCT FROM EXCLUDED."lastSampleDate"
			THEN COALESCE(u."storageSampleCount", 0) + 1
			ELSE COALESCE(u."storageSampleCount", 1) END`
)

func (j *Job) upsertSample(ctx context.Context, a area, year, month int, day time.Time, library *bunny.VideoLibrary, pullZoneID string) error {
	_, err := j.db.ExecContext(ctx, sqlUpsertSample,
		a.tenantID, year, month,
		library.TrafficUsage,
		library.StorageUsage,
		day,
		a.libraryID,
		pullZoneID,
	)
	return err
}

// sqlUpsertMissing records an area whose library Bunny does not have.
//
// It writes no number. The insert takes the column default of 0 for traffic
// because a row has to have one, and `source` is what says that 0 is not a
// measurement; on an existing row nothing but the marker is touched, so
// yesterday's real numbers are not overwritten with zero by today's 404.
const sqlUpsertMissing = `
	INSERT INTO "TenantBunnyMonthlyUsage" AS u (
		"tenantId", year, month, source, "bunnyLibraryId", "syncedAt"
	)
	VALUES ($1, $2, $3, '` + sourceMissing + `', $4, now())
	ON CONFLICT ("tenantId", year, month) DO UPDATE SET
		source           = '` + sourceMissing + `',
		"bunnyLibraryId" = EXCLUDED."bunnyLibraryId",
		"syncedAt"       = now()
	WHERE u."closedAt" IS NULL
`

func (j *Job) upsertMissing(ctx context.Context, a area, year, month int) error {
	_, err := j.db.ExecContext(ctx, sqlUpsertMissing, a.tenantID, year, month, a.libraryID)
	return err
}

// sqlMirrorToTenant keeps the three columns the manager reads today in step
// with the row just written. The pull zone rides along: it is resolved from the
// same response, and having it stored is what lets the closing pass reach
// /statistics for an area whose library has since gone missing.
const sqlMirrorToTenant = `
	UPDATE "Tenant"
	SET "bunnyStorageBytes"   = $2,
	    "bunnyTrafficBytes"   = $3,
	    "bunnyUsageUpdatedAt" = now(),
	    "bunnyPullZoneId"     = COALESCE(NULLIF($4::text, ''), "bunnyPullZoneId")
	WHERE id = $1
`

func (j *Job) mirrorToTenant(ctx context.Context, tenantID string, library *bunny.VideoLibrary, pullZoneID string) error {
	_, err := j.db.ExecContext(ctx, sqlMirrorToTenant,
		tenantID, library.StorageUsage, library.TrafficUsage, pullZoneID)
	return err
}
