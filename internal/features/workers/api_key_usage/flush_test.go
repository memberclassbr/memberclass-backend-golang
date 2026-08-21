package api_key_usage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// fakeStore is a Redis whose contents a test writes directly.
type fakeStore struct {
	hashes  map[string]map[string]string
	locked  map[string]bool
	deleted []string
	lockErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{hashes: map[string]map[string]string{}, locked: map[string]bool{}}
}

func (s *fakeStore) HGetAll(_ context.Context, key string) (map[string]string, error) {
	return s.hashes[key], nil
}

func (s *fakeStore) SetNX(_ context.Context, key string, _ string, _ time.Duration) (bool, error) {
	if s.lockErr != nil {
		return false, s.lockErr
	}
	if s.locked[key] {
		return false, nil
	}
	s.locked[key] = true
	return true, nil
}

func (s *fakeStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.hashes, key)
	return nil
}

// The clock is pinned away from the prune hour so a test only exercises the
// flush; the prune has its own test.
var fixedNow = time.Date(2026, 8, 21, 13, 5, 0, 0, time.UTC)

func newTestJob(t *testing.T, store Store) (*Job, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	job := New(db, store, fakeLogger{}, "replica-1")
	job.now = func() time.Time { return fixedNow }
	return job, mock, db
}

// The happy path: one field becomes one row, with the tenant resolved from the
// key and lastUsedAt stamped in the same pass.
func TestExecute_WritesTheDaysCountersAndStampsLastUsed(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-21"] = map[string]string{"key-1|user/informations": "42"}
	store.hashes["apikey:usage:err:2026-08-21"] = map[string]string{"key-1|user/informations": "3"}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenantId"}).AddRow("key-1", "t1"))
	mock.ExpectExec(`INSERT INTO "ApiKeyUsageDaily"`).
		WithArgs(time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC), "t1", "key-1", "user/informations", int64(42), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "TenantApiKey"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The current day stays in Redis, because it is still being counted. Deleting
// it would throw away every request since the last flush.
func TestExecute_KeepsTodaysCountersInRedis(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-21"] = map[string]string{"key-1|user/informations": "1"}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenantId"}).AddRow("key-1", "t1"))
	mock.ExpectExec(`INSERT INTO "ApiKeyUsageDaily"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "TenantApiKey"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.Empty(t, store.deleted)
}

// A day that has ended is written and then cleared, which is what stops the
// hash living out its TTL being re-read every hour.
func TestExecute_ClearsAClosedDay(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-20"] = map[string]string{"key-1|user/informations": "7"}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenantId"}).AddRow("key-1", "t1"))
	mock.ExpectExec(`INSERT INTO "ApiKeyUsageDaily"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "TenantApiKey"`).WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, job.Execute(context.Background()))
	assert.Equal(t, []string{"apikey:usage:req:2026-08-20", "apikey:usage:err:2026-08-20"}, store.deleted)
}

// The lock is what stops two replicas draining the same hash. The one that
// loses it must not read, must not write, and must not delete.
func TestExecute_DoesNothingWithoutTheLock(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-21"] = map[string]string{"key-1|user/informations": "9"}
	for _, day := range []string{"2026-08-21", "2026-08-20", "2026-08-19"} {
		store.locked["apikey:usage:flush:"+day] = true
	}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	require.NoError(t, job.Execute(context.Background()))
	assert.Empty(t, store.deleted)
	// No query ran at all.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A key deleted in the panel takes its usage rows with it; a flush must not
// resurrect them.
func TestExecute_SkipsCountersForADeletedKey(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-21"] = map[string]string{"gone|user/informations": "5"}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenantId"}))

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An error count without a request count is not data. Errors are a subset of
// requests, so the pair is the only shape that can be written.
func TestExecute_IgnoresAnErrorCountWithNoRequests(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:err:2026-08-21"] = map[string]string{"key-1|user/informations": "4"}

	job, _, db := newTestJob(t, store)
	defer db.Close()

	require.NoError(t, job.Execute(context.Background()))
}

// One failing day must not stop the next: the scan is three independent days.
func TestExecute_ReportsAFailureAndKeepsGoing(t *testing.T) {
	store := newFakeStore()
	store.hashes["apikey:usage:req:2026-08-21"] = map[string]string{"key-1|a": "1"}
	store.hashes["apikey:usage:req:2026-08-20"] = map[string]string{"key-1|b": "1"}

	job, mock, db := newTestJob(t, store)
	defer db.Close()

	mock.ExpectQuery(`FROM "TenantApiKey"`).WillReturnError(errors.New("boom"))
	mock.ExpectQuery(`FROM "TenantApiKey"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenantId"}).AddRow("key-1", "t1"))
	mock.ExpectExec(`INSERT INTO "ApiKeyUsageDaily"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "TenantApiKey"`).WillReturnResult(sqlmock.NewResult(0, 1))

	assert.Error(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Retention runs once a day, not once an hour: the table's index leads with
// tenantId, so a delete filtered on date alone scans.
func TestExecute_PrunesOnlyAtThePruneHour(t *testing.T) {
	store := newFakeStore()
	job, mock, db := newTestJob(t, store)
	defer db.Close()

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet(), "13:05 UTC is not the prune hour")

	job.now = func() time.Time { return time.Date(2026, 8, 21, pruneHourUTC, 5, 0, 0, time.UTC) }
	mock.ExpectExec(`DELETE FROM "ApiKeyUsageDaily"`).
		WithArgs(time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)).
		WillReturnResult(sqlmock.NewResult(0, 12))

	require.NoError(t, job.Execute(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}
