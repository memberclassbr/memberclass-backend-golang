package bunny_usage

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// fakeStore is the day lock. locked says the day is already taken.
type fakeStore struct {
	locked bool
	err    error
	keys   []string
}

func (s *fakeStore) SetNX(_ context.Context, key string, _ string, _ time.Duration) (bool, error) {
	s.keys = append(s.keys, key)
	if s.err != nil {
		return false, s.err
	}
	return !s.locked, nil
}

// fakeBunny answers per library / pull zone, and records what it was asked.
type fakeBunny struct {
	libraries  map[string]*bunny.VideoLibrary
	libraryErr map[string]error

	statistics  map[string]int64
	statsErr    map[string]error
	statsCalls  []statsCall
	libraryHits []string
}

type statsCall struct {
	pullZoneID string
	from, to   string
}

func (f *fakeBunny) GetVideoLibrary(_ context.Context, libraryID string) (*bunny.VideoLibrary, error) {
	f.libraryHits = append(f.libraryHits, libraryID)
	if err, ok := f.libraryErr[libraryID]; ok {
		return nil, err
	}
	if library, ok := f.libraries[libraryID]; ok {
		return library, nil
	}
	return nil, bunny.ErrLibraryNotFound
}

func (f *fakeBunny) GetStatistics(_ context.Context, pullZoneID string, from, to time.Time) (*bunny.Statistics, error) {
	f.statsCalls = append(f.statsCalls, statsCall{pullZoneID, from.Format("2006-01-02"), to.Format("2006-01-02")})
	if err, ok := f.statsErr[pullZoneID]; ok {
		return nil, err
	}
	return &bunny.Statistics{TotalBandwidthUsed: f.statistics[pullZoneID]}, nil
}

// A mid-month day: the closing pass has nothing to do unless a test gives it
// an open past month of its own.
var fixedNow = time.Date(2026, 9, 15, 5, 30, 0, 0, time.UTC)

func newTestJob(t *testing.T, account bunny.AccountService, store Store) (*Job, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	job := New(db, account, store, fakeLogger{}, "replica-1")
	job.now = func() time.Time { return fixedNow }
	job.throttle = 0
	return job, mock, db
}

func expectAreas(mock sqlmock.Sqlmock, areas ...area) {
	rows := sqlmock.NewRows([]string{"id", "bunnyLibraryId", "bunnyPullZoneId"})
	for _, a := range areas {
		rows.AddRow(a.tenantID, a.libraryID, a.pullZoneID)
	}
	mock.ExpectQuery(`FROM "Tenant"`).WillReturnRows(rows)
}

// expectNoOpenMonths is the closing pass finding nothing to do.
func expectNoOpenMonths(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM "TenantBunnyMonthlyUsage" u`).
		WillReturnRows(sqlmock.NewRows([]string{"tenantId", "year", "month", "bunnyPullZoneId"}))
}

// The happy path: one library read becomes one month row plus the three Tenant
// columns the manager still reads.
func TestExecute_SamplesTheCurrentMonthAndMirrorsToTenant(t *testing.T) {
	account := &fakeBunny{libraries: map[string]*bunny.VideoLibrary{
		"lib-1": {ID: 1, PullZoneID: 3697175, StorageUsage: 900, TrafficUsage: 129375439},
	}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-1"})
	expectNoOpenMonths(mock)

	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).
		WithArgs("tenant-1", 2026, 9, int64(129375439), int64(900),
			time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), "lib-1", "3697175").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE "Tenant"`).
		WithArgs("tenant-1", int64(900), int64(129375439), "3697175").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A 404 is an expected, per-area condition — the library is gone from Bunny and
// the column here has not caught up. It is recorded as unknown usage and does
// not count as a failure, or the daily noise would drown the alert that matters.
func TestExecute_MissingLibraryIsRecordedNotFailed(t *testing.T) {
	account := &fakeBunny{libraryErr: map[string]error{"lib-gone": bunny.ErrLibraryNotFound}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock, area{tenantID: "tenant-1", libraryID: "lib-gone"})
	expectNoOpenMonths(mock)

	mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).
		WithArgs("tenant-1", 2026, 9, "lib-gone").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// No Tenant mirror: there is no number to mirror.
	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// One rejected account key fails every area, so the run stops on the first one
// rather than spending a request per area to learn the same thing.
func TestExecute_UnauthorizedAbortsTheRun(t *testing.T) {
	account := &fakeBunny{libraryErr: map[string]error{"lib-1": bunny.ErrUnauthorized}}
	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	expectAreas(mock,
		area{tenantID: "tenant-1", libraryID: "lib-1"},
		area{tenantID: "tenant-2", libraryID: "lib-2"},
	)
	expectNoOpenMonths(mock)

	err := job.Execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, bunny.ErrUnauthorized)
	assert.Equal(t, []string{"lib-1"}, account.libraryHits, "the second area should not have been tried")
}

// Below the threshold a failed area is a warning: traffic is overwritten from a
// running total tomorrow, and one missed storage sample moves the month's
// average by one reading.
func TestExecute_AFewFailingAreasDoNotFailTheRun(t *testing.T) {
	account := &fakeBunny{
		libraries:  map[string]*bunny.VideoLibrary{},
		libraryErr: map[string]error{"lib-1": errors.New("connection reset")},
	}
	for i := 2; i <= 10; i++ {
		account.libraries[libID(i)] = &bunny.VideoLibrary{PullZoneID: i, StorageUsage: 1, TrafficUsage: 2}
	}

	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	areas := []area{{tenantID: "tenant-1", libraryID: "lib-1"}}
	for i := 2; i <= 10; i++ {
		areas = append(areas, area{tenantID: tenantID(i), libraryID: libID(i)})
	}
	expectAreas(mock, areas...)
	expectNoOpenMonths(mock)

	for i := 2; i <= 10; i++ {
		mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE "Tenant"`).WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// 1 of 10 is under the 20% threshold.
	require.NoError(t, job.Execute(context.Background()))
}

// Above it, the cause is not the areas.
func TestExecute_ManyFailingAreasFailTheRun(t *testing.T) {
	account := &fakeBunny{libraries: map[string]*bunny.VideoLibrary{}, libraryErr: map[string]error{}}
	for i := 1; i <= 10; i++ {
		if i <= 3 {
			account.libraryErr[libID(i)] = errors.New("connection reset")
			continue
		}
		account.libraries[libID(i)] = &bunny.VideoLibrary{PullZoneID: i, StorageUsage: 1, TrafficUsage: 2}
	}

	job, mock, db := newTestJob(t, account, &fakeStore{})
	defer db.Close()

	var areas []area
	for i := 1; i <= 10; i++ {
		areas = append(areas, area{tenantID: tenantID(i), libraryID: libID(i)})
	}
	expectAreas(mock, areas...)
	expectNoOpenMonths(mock)

	for i := 4; i <= 10; i++ {
		mock.ExpectExec(`INSERT INTO "TenantBunnyMonthlyUsage"`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE "Tenant"`).WillReturnResult(sqlmock.NewResult(0, 1))
	}

	err := job.Execute(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 of 10")
}

// Every replica starts the scheduler. Two of them walking a hundred libraries
// is the account's rate limit spent twice for one set of numbers.
func TestExecute_SkipsWhenAnotherReplicaHoldsTheDay(t *testing.T) {
	account := &fakeBunny{}
	job, mock, db := newTestJob(t, account, &fakeStore{locked: true})
	defer db.Close()

	require.NoError(t, job.Execute(context.Background()))
	assert.Empty(t, account.libraryHits)
	assert.NoError(t, mock.ExpectationsWereMet(), "no query should have run")
}

func TestExecute_LockKeyIsTheUTCDay(t *testing.T) {
	store := &fakeStore{locked: true}
	job, _, db := newTestJob(t, &fakeBunny{}, store)
	defer db.Close()

	require.NoError(t, job.Execute(context.Background()))
	assert.Equal(t, []string{"bunny:usage:sync:2026-09-15"}, store.keys)
}

func libID(i int) string    { return "lib-" + strconv.Itoa(i) }
func tenantID(i int) string { return "tenant-" + strconv.Itoa(i) }
