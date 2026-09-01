package bunny_usage

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

func expectExistingMonths(mock sqlmock.Sqlmock, months ...[2]int) {
	rows := sqlmock.NewRows([]string{"year", "month"})
	for _, m := range months {
		rows.AddRow(m[0], m[1])
	}
	mock.ExpectQuery(`FROM "TenantBunnyMonthlyUsage"`).WillReturnRows(rows)
}

// A backfilled row carries traffic and nothing else: /statistics has no storage
// field, so every storage column stays null — "not measured", not "measured
// zero". The insert names only the columns it can fill, which is what leaves
// them that way.
func TestBackfill_WritesTrafficOnlyForTheMonthsBefore(t *testing.T) {
	account := &fakeBunny{statistics: map[string]int64{"3697175": 5000}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1", pullZoneID: "3697175"})
	expectExistingMonths(mock)

	for i := 0; i < 2; i++ {
		mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	require.NoError(t, job.Backfill(context.Background(), 2, ""))
	assert.NoError(t, mock.ExpectationsWereMet())

	// Most recent first, and never the current month — that one is still being
	// measured and is not final.
	require.Len(t, account.statsCalls, 2)
	assert.Equal(t, statsCall{"3697175", "2026-08-01", "2026-08-31"}, account.statsCalls[0])
	assert.Equal(t, statsCall{"3697175", "2026-07-01", "2026-07-31"}, account.statsCalls[1])
}

// A month this worker already measured is better than anything a
// reconstruction can offer, and the request is skipped before it is spent
// rather than after.
func TestBackfill_SkipsMonthsThatAlreadyHaveARow(t *testing.T) {
	account := &fakeBunny{statistics: map[string]int64{"3697175": 5000}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1", pullZoneID: "3697175"})
	expectExistingMonths(mock, [2]int{2026, 8})

	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Backfill(context.Background(), 2, ""))
	assert.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, account.statsCalls, 1)
	assert.Equal(t, "2026-07-01", account.statsCalls[0].from)
}

// Bunny keeps one rolling year, so the API refuses anything older and the
// backfill leaves no row: an absent row reads as "we don't know", and a zeroed
// one would be a claim that nothing was used.
func TestBackfill_LeavesNoRowBeyondTheRetention(t *testing.T) {
	account := &fakeBunny{
		statistics: map[string]int64{"3697175": 5000},
		statsErr:   map[string]error{"3697175": bunny.ErrStatisticsOutOfRange},
	}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1", pullZoneID: "3697175"})
	expectExistingMonths(mock)

	// No insert is expected at all, and the area stops asking after the first
	// refusal — everything older is older still.
	require.NoError(t, job.Backfill(context.Background(), 12, ""))
	assert.Len(t, account.statsCalls, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A pull zone is resolved once per area and stored, because a backfill is one
// call per month per zone and the resolution should not be paid twelve times.
func TestBackfill_ResolvesAndStoresAMissingPullZone(t *testing.T) {
	account := &fakeBunny{
		libraries:  map[string]*bunny.VideoLibrary{"lib-1": {PullZoneID: 3697175}},
		statistics: map[string]int64{"3697175": 10},
	}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1"})

	mock.ExpectExec(`UPDATE "Tenant"`).
		WithArgs("tenant-1", "3697175").
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectExistingMonths(mock)
	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Backfill(context.Background(), 1, ""))
	assert.Equal(t, []string{"lib-1"}, account.libraryHits)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBackfill_ClampsToTheRetentionWindow(t *testing.T) {
	account := &fakeBunny{statistics: map[string]int64{"3697175": 1}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1", pullZoneID: "3697175"})
	expectExistingMonths(mock)
	for i := 0; i < MaxBackfillMonths; i++ {
		mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// Asking for two years gets one, because a thirteenth month has no source.
	require.NoError(t, job.Backfill(context.Background(), 24, ""))
	assert.Len(t, account.statsCalls, MaxBackfillMonths)
}
