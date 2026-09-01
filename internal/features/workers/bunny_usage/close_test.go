package bunny_usage

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

func expectOpenMonths(mock sqlmock.Sqlmock, months ...openMonth) {
	rows := sqlmock.NewRows([]string{"tenantId", "year", "month", "bunnyPullZoneId"})
	for _, m := range months {
		rows.AddRow(m.tenantID, m.year, m.month, m.pullZoneID)
	}
	mock.ExpectQuery(`FROM "TenantBunnyMonthlyUsage" u`).WillReturnRows(rows)
}

// A finished month is written from the period total, never from the library's
// own counter: Bunny resets `TrafficUsage` at the UTC turn of the 1st, so a run
// on the 1st at 05:30 UTC would read a counter six hours into the new month and
// lose the last ~21 hours of the old one.
func TestExecute_ClosesAFinishedMonthFromTheStatisticsPeriod(t *testing.T) {
	account := &fakeBunny{
		libraries:  map[string]*bunny.VideoLibrary{"lib-1": {PullZoneID: 3697175, StorageUsage: 900, TrafficUsage: 1}},
		statistics: map[string]int64{"3697175": 129377181},
	}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1", pullZoneID: "3697175"})
	expectOpenMonths(mock, openMonth{tenantID: "tenant-1", year: 2026, month: 8, pullZoneID: "3697175"})

	mock.ExpectExec(`UPDATE "TenantBunnyMonthlyUsage"`).
		WithArgs("tenant-1", 2026, 8, int64(129377181), "3697175").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Then the current month is sampled as usual.
	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "Tenant"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())

	// The whole of August, inclusive on both ends — the closed period, not the
	// month to date.
	require.Len(t, account.statsCalls, 1)
	assert.Equal(t, statsCall{"3697175", "2026-08-01", "2026-08-31"}, account.statsCalls[0])
}

// Without a pull zone there is no way to reach /statistics. The month stays
// open and is retried once a daily pass has read the library and stored one,
// rather than being closed from a number nobody has.
func TestExecute_LeavesAMonthWithNoPullZoneOpen(t *testing.T) {
	account := &fakeBunny{libraries: map[string]*bunny.VideoLibrary{"lib-1": {PullZoneID: 7, StorageUsage: 1, TrafficUsage: 2}}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1"})
	expectOpenMonths(mock, openMonth{tenantID: "tenant-1", year: 2026, month: 8})

	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "Tenant"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.Empty(t, account.statsCalls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The scan is bounded by what /statistics answers for. A month older than the
// retention can never be closed, so looking for it would only produce the same
// failure every day forever.
func TestListOpenFinishedMonths_ScansOnlyBackToTheRetentionLimit(t *testing.T) {
	job, mock, db := newTestJob(t, &fakeBunny{}, &fakeStore{})
	defer db.Close()

	current := 2026*12 + 9
	mock.ExpectQuery(`FROM "TenantBunnyMonthlyUsage" u`).
		WithArgs(current, current-closeLookbackMonths).
		WillReturnRows(sqlmock.NewRows([]string{"tenantId", "year", "month", "bunnyPullZoneId"}))

	_, err := job.listOpenFinishedMonths(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Reprocessing a closed month is the one write that ignores closedAt, which is
// why nothing scheduled can reach it.
func TestReopenAndCloseMonth_OverwritesAClosedMonth(t *testing.T) {
	account := &fakeBunny{statistics: map[string]int64{"3697175": 999}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantBunnyMonthlyUsage" u`).
		WithArgs("tenant-1", 2026, 7).
		WillReturnRows(sqlmock.NewRows([]string{"bunnyPullZoneId"}).AddRow("3697175"))

	mock.ExpectExec(`UPDATE "TenantBunnyMonthlyUsage"`).
		WithArgs("tenant-1", 2026, 7, int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.ReopenAndCloseMonth(context.Background(), "tenant-1", 2026, 7))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMonthBounds_IsTheWholeMonthInclusive(t *testing.T) {
	cases := map[string][2]string{
		"February in a leap year": {"2024-02-01", "2024-02-29"},
		"February otherwise":      {"2026-02-01", "2026-02-28"},
		"December":                {"2026-12-01", "2026-12-31"},
	}
	for name, want := range cases {
		start, _ := time.Parse("2006-01-02", want[0])
		from, to := monthBounds(start.Year(), int(start.Month()))

		assert.Equal(t, want[0], from.Format("2006-01-02"), name)
		assert.Equal(t, want[1], to.Format("2006-01-02"), name)
	}
}
