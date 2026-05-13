package transcription

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func TestClaimPending_HappyPath(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	f := &Feature{transcriptionDB: db, log: logger.NewLogger()}

	mock.ExpectQuery(`UPDATE jobs.*RETURNING id, tenant_id, payload, attempts, max_attempts`).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "payload", "attempts", "max_attempts"}).
			AddRow("job-1", "tenant-a", []byte(`{}`), 0, 3).
			AddRow("job-2", "tenant-b", []byte(`{}`), 1, 3))

	jobs, err := f.claimPending(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].ID != "job-1" || jobs[1].Attempts != 1 {
		t.Fatalf("bad scan: %+v", jobs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimPending_ZeroLimit(t *testing.T) {
	f := &Feature{}
	jobs, err := f.claimPending(context.Background(), 0)
	if err != nil || jobs != nil {
		t.Fatalf("expected nil/nil, got %+v / %v", jobs, err)
	}
}

func TestMarkJobFailed_FlipsStatus(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	f := &Feature{transcriptionDB: db, log: logger.NewLogger()}

	mock.ExpectQuery(`UPDATE jobs.*RETURNING status`).
		WithArgs("j", "boom").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("PENDING"))

	if err := f.markJobFailed(context.Background(), "j", "boom"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResetOrphans_ReturnsCount(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	f := &Feature{transcriptionDB: db, log: logger.NewLogger()}

	mock.ExpectQuery(`UPDATE jobs.*status = 'PENDING'.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("o1").AddRow("o2"))

	n, err := f.resetOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("count=%d, want 2", n)
	}
}

func TestStartStop_IsIdempotent(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// Long poll interval so the run loop doesn't actually claim before we
	// Stop — keeps the test from racing the sqlmock with no expectations.
	f := &Feature{
		transcriptionDB: db,
		log:             logger.NewLogger(),
		pollInterval:    24 * time.Hour,
		workers:         1,
	}

	ctx := context.Background()
	f.Start(ctx)
	f.Start(ctx) // 2nd Start = no-op (still one running loop)

	// Yield to give the run goroutine a chance to enter the select.
	time.Sleep(10 * time.Millisecond)

	f.Stop(2 * time.Second)
	f.Stop(2 * time.Second) // 2nd Stop also a no-op
}
