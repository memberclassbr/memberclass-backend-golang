package bunny_usage

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

// The rest of this package's tests run against go-sqlmock, which matches a
// query without executing it — so it says nothing about whether these
// statements parse, or whether the storage accumulator computes the right
// average. That arithmetic is the part of this worker with no second chance:
// traffic is re-read from a running total every day, and a month's storage
// exists only as the samples taken during it.
//
// Point BUNNY_USAGE_TEST_DSN at a throwaway Postgres to run it:
//
//	docker run -d --rm -e POSTGRES_PASSWORD=x -p 55439:5432 postgres:16-alpine
//	BUNNY_USAGE_TEST_DSN="postgres://postgres:x@localhost:55439/postgres?sslmode=disable" \
//	  go test ./internal/features/workers/bunny_usage/ -run TestSQL
//
// The schema mirrors the Prisma models in the sibling mult-memberclass repo,
// which owns them; this repository only reads and writes those tables.
const testSchema = `
DROP TABLE IF EXISTS "TenantBunnyMonthlyUsage";
DROP TABLE IF EXISTS "Tenant";
CREATE TABLE "Tenant" (
  id                    TEXT PRIMARY KEY,
  "bunnyLibraryId"      TEXT,
  "bunnyPullZoneId"     TEXT,
  "bunnyStorageBytes"   BIGINT,
  "bunnyTrafficBytes"   BIGINT,
  "bunnyUsageUpdatedAt" TIMESTAMPTZ
);
CREATE TABLE "TenantBunnyMonthlyUsage" (
  "tenantId"           TEXT NOT NULL REFERENCES "Tenant"(id) ON DELETE CASCADE,
  year                 INT NOT NULL,
  month                INT NOT NULL,
  "trafficBytes"       BIGINT NOT NULL DEFAULT 0,
  "storageAvgBytes"    BIGINT,
  "storageLastBytes"   BIGINT,
  "storageSampleSum"   BIGINT,
  "storageSampleCount" INT,
  "lastSampleDate"     DATE,
  source               TEXT NOT NULL DEFAULT 'measured',
  "bunnyLibraryId"     TEXT,
  "bunnyPullZoneId"    TEXT,
  "closedAt"           TIMESTAMPTZ,
  "syncedAt"           TIMESTAMPTZ,
  PRIMARY KEY ("tenantId", year, month)
);
INSERT INTO "Tenant" (id, "bunnyLibraryId") VALUES ('t1', 'lib-1');
`

type usageRow struct {
	traffic     int64
	avg         sql.NullInt64
	last        sql.NullInt64
	sum         sql.NullInt64
	sampleCount sql.NullInt64
	lastSample  sql.NullTime
	source      string
	closedAt    sql.NullTime
}

func readUsage(t *testing.T, db *sql.DB, year, month int) usageRow {
	t.Helper()

	var r usageRow
	err := db.QueryRow(`
		SELECT "trafficBytes", "storageAvgBytes", "storageLastBytes", "storageSampleSum",
		       "storageSampleCount", "lastSampleDate", source, "closedAt"
		FROM "TenantBunnyMonthlyUsage"
		WHERE "tenantId" = 't1' AND year = $1 AND month = $2`, year, month).
		Scan(&r.traffic, &r.avg, &r.last, &r.sum, &r.sampleCount, &r.lastSample, &r.source, &r.closedAt)
	require.NoError(t, err)
	return r
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("BUNNY_USAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("set BUNNY_USAGE_TEST_DSN to run the SQL integration test")
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.Ping())
	_, err = db.Exec(testSchema)
	require.NoError(t, err)
	return db
}

func TestSQL_StorageIsSampledOncePerDayAndTrafficIsOverwritten(t *testing.T) {
	db := openTestDB(t)
	job := &Job{db: db, log: fakeLogger{}}
	a := area{tenantID: "t1", libraryID: "lib-1"}
	ctx := context.Background()

	day1 := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)

	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, day1,
		&bunny.VideoLibrary{StorageUsage: 100, TrafficUsage: 10}, "77"))

	row := readUsage(t, db, 2026, 9)
	require.Equal(t, int64(10), row.traffic)
	require.Equal(t, int64(100), row.avg.Int64)
	require.Equal(t, int64(1), row.sampleCount.Int64)

	// A second run on the same UTC day. Traffic is re-read from Bunny's running
	// total, so it moves; the day's storage sample is already taken, and adding
	// it again would weight one day double in the month's average.
	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, day1,
		&bunny.VideoLibrary{StorageUsage: 200, TrafficUsage: 20}, "77"))

	row = readUsage(t, db, 2026, 9)
	require.Equal(t, int64(20), row.traffic)
	require.Equal(t, int64(200), row.last.Int64, "the current reading still moves")
	require.Equal(t, int64(1), row.sampleCount.Int64)
	require.Equal(t, int64(100), row.avg.Int64, "the average keeps the day's one sample")

	// The next day contributes a sample of its own.
	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, day2,
		&bunny.VideoLibrary{StorageUsage: 400, TrafficUsage: 30}, "77"))

	row = readUsage(t, db, 2026, 9)
	require.Equal(t, int64(2), row.sampleCount.Int64)
	require.Equal(t, int64(500), row.sum.Int64)
	require.Equal(t, int64(250), row.avg.Int64)
}

// A library Bunny answered 404 for has a number nobody knows. Marking the row
// must not overwrite the numbers the month already measured with zero.
func TestSQL_AMissingLibraryDoesNotZeroTheMonth(t *testing.T) {
	db := openTestDB(t)
	job := &Job{db: db, log: fakeLogger{}}
	a := area{tenantID: "t1", libraryID: "lib-1"}
	ctx := context.Background()

	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
		&bunny.VideoLibrary{StorageUsage: 400, TrafficUsage: 30}, "77"))
	require.NoError(t, job.upsertMissing(ctx, a, 2026, 9))

	row := readUsage(t, db, 2026, 9)
	require.Equal(t, sourceMissing, row.source)
	require.Equal(t, int64(30), row.traffic, "an absent library is unknown usage, not zero")
	require.Equal(t, int64(400), row.last.Int64)
}

// closedAt is the rewrite lock. The daily pass must not touch a figure a
// customer has already been billed from.
func TestSQL_AClosedMonthIsNotRewrittenByTheDailyPass(t *testing.T) {
	db := openTestDB(t)
	job := &Job{db: db, log: fakeLogger{}}
	a := area{tenantID: "t1", libraryID: "lib-1"}
	ctx := context.Background()
	day := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)

	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, day,
		&bunny.VideoLibrary{StorageUsage: 400, TrafficUsage: 30}, "77"))

	_, err := db.Exec(sqlCloseMonth, "t1", 2026, 9, int64(999), "77")
	require.NoError(t, err)

	require.NoError(t, job.upsertSample(ctx, a, 2026, 9, day.AddDate(0, 0, 1),
		&bunny.VideoLibrary{StorageUsage: 1, TrafficUsage: 1}, "77"))
	require.NoError(t, job.upsertMissing(ctx, a, 2026, 9))

	row := readUsage(t, db, 2026, 9)
	require.True(t, row.closedAt.Valid)
	require.Equal(t, int64(999), row.traffic)
	require.Equal(t, int64(400), row.last.Int64)

	// The deliberate manual reprocess is the one write that ignores it.
	_, err = db.Exec(sqlReopenAndCloseMonth, "t1", 2026, 9, int64(4242))
	require.NoError(t, err)
	require.Equal(t, int64(4242), readUsage(t, db, 2026, 9).traffic)
}

// /statistics has no storage field, so a reconstructed month has to say "not
// measured" rather than "measured zero" — and it says it by leaving every
// storage column null.
func TestSQL_ABackfilledMonthCarriesTrafficAndNoStorage(t *testing.T) {
	db := openTestDB(t)
	job := &Job{db: db, log: fakeLogger{}}
	ctx := context.Background()

	require.NoError(t, job.insertBackfilled(ctx, area{tenantID: "t1", libraryID: "lib-1"}, 2026, 7, 5000, "77"))

	row := readUsage(t, db, 2026, 7)
	require.Equal(t, int64(5000), row.traffic)
	require.Equal(t, sourceBackfilled, row.source)
	require.True(t, row.closedAt.Valid, "a past month is already final")
	require.False(t, row.avg.Valid)
	require.False(t, row.last.Valid)
	require.False(t, row.sum.Valid)
	require.False(t, row.sampleCount.Valid)
	require.False(t, row.lastSample.Valid)
}

// The statements with no assertion of their own still have to parse and run.
func TestSQL_EveryQueryRuns(t *testing.T) {
	db := openTestDB(t)
	job := &Job{db: db, log: fakeLogger{}}
	ctx := context.Background()

	areas, err := job.listAreas(ctx)
	require.NoError(t, err)
	require.Len(t, areas, 1)

	require.NoError(t, job.upsertSample(ctx, areas[0], 2026, 8,
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		&bunny.VideoLibrary{StorageUsage: 1, TrafficUsage: 2}, "77"))
	require.NoError(t, job.mirrorToTenant(ctx, "t1", &bunny.VideoLibrary{StorageUsage: 1, TrafficUsage: 2}, "77"))

	months, err := job.listOpenFinishedMonths(ctx, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, []openMonth{{tenantID: "t1", year: 2026, month: 8, pullZoneID: "77"}}, months)

	have, err := job.existingMonths(ctx, "t1")
	require.NoError(t, err)
	require.Contains(t, have, 2026*12+8)

	_, err = db.Exec(sqlStorePullZone, "t1", "88")
	require.NoError(t, err)

	var pullZoneID string
	require.NoError(t, db.QueryRow(sqlPullZoneForMonth, "t1", 2026, 8).Scan(&pullZoneID))
	require.Equal(t, "77", pullZoneID)
}
