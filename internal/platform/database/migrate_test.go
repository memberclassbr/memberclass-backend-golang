package database

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

type fakeLogger struct{ infos []string }

func (l *fakeLogger) Debug(string, ...any)      {}
func (l *fakeLogger) Info(msg string, _ ...any) { l.infos = append(l.infos, msg) }
func (l *fakeLogger) Warn(string, ...any)       {}
func (l *fakeLogger) Error(string, ...any)      {}
func (l *fakeLogger) said(substr string) bool {
	for _, m := range l.infos {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// The embedded transcription migrations are handed to the server as SQL by
// MigrateTranscription — they are never fed to psql, and they never own their
// own transaction. Both were true of an earlier life where these files were run
// by hand with `psql -f`, and the leftovers only surfaced at boot on a
// deployment that actually has DB_TRANSCRIPTION_DSN set. These tests are cheap
// and need no database, so CI catches the next one instead of a crashed deploy.

// lineComments strips `-- ...` so prose in a header cannot trip the checks.
var lineComments = regexp.MustCompile(`(?m)--.*$`)

// singleQuoted strips '...' literals for the same reason.
var singleQuoted = regexp.MustCompile(`'[^']*'`)

func migrationBodies(t *testing.T) map[string]string {
	t.Helper()

	names, err := migrationNames()
	if err != nil {
		t.Fatalf("listing embedded migrations: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no embedded migrations found; the go:embed pattern is not matching")
	}

	bodies := make(map[string]string, len(names))
	for _, name := range names {
		raw, err := transcriptionMigrations.ReadFile("migrations/transcription/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		bodies[name] = singleQuoted.ReplaceAllString(lineComments.ReplaceAllString(string(raw), ""), "''")
	}
	return bodies
}

// A psql meta-command is not SQL. `SELECT ... \gset` at the end of a statement
// in 000_dedupe_videos.sql took a deployment down at boot with
// `pq: syntax error at or near "\"` — and because the whole file is one batch,
// nothing in it ran and no later migration ran either.
//
// Backslashes are checked anywhere, not only at the start of a line: \gset
// trails its query on the same line, so a line-anchored check would have missed
// exactly the bug that caused the outage. If a migration ever needs a literal
// backslash outside a quoted string, relax this deliberately.
func TestMigrations_ContainNoPsqlMetaCommands(t *testing.T) {
	for name, body := range migrationBodies(t) {
		if idx := strings.Index(body, `\`); idx >= 0 {
			t.Errorf("%s contains a backslash at offset %d — psql meta-commands are not SQL and fail at boot: %q",
				name, idx, excerpt(body, idx))
		}
	}
}

// applyMigration already wraps each file in a transaction and writes the
// schema_migrations_go row inside it. A COMMIT in the file ends that
// transaction early, so the bookkeeping row lands outside it and the
// "a failure leaves nothing half-applied" guarantee stops holding.
//
// The pattern requires the semicolon right after the keyword, so a PL/pgSQL
// `DO $$ BEGIN ... END $$;` block is not caught by it.
var txnControl = regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT|ROLLBACK|START\s+TRANSACTION)\s*;`)

func TestMigrations_DoNotOpenTheirOwnTransaction(t *testing.T) {
	for name, body := range migrationBodies(t) {
		if found := txnControl.FindString(body); found != "" {
			t.Errorf("%s issues %q — MigrateTranscription owns the transaction; committing here drops the row for this file outside it",
				name, strings.TrimSpace(found))
		}
	}
}

// Order is filename order, which is what the numeric prefixes are for. A
// duplicate prefix would make the order depend on the rest of the name.
func TestMigrationNames_AreOrderedAndUniquelyPrefixed(t *testing.T) {
	names, err := migrationNames()
	if err != nil {
		t.Fatalf("listing embedded migrations: %v", err)
	}

	seen := make(map[string]string, len(names))
	for i, name := range names {
		if i > 0 && names[i-1] >= name {
			t.Errorf("migrations are not sorted: %q comes before %q", names[i-1], name)
		}

		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("%s has no numeric prefix; order is filename order", name)
			continue
		}
		if other, dup := seen[prefix]; dup {
			t.Errorf("%s and %s share the prefix %q", other, name, prefix)
		}
		seen[prefix] = name
	}
}

// Every migration needs a probe, so that adding one forces the question "how
// would I recognise this as already applied?" to be answered while the artefact
// is fresh in mind rather than during an incident.
func TestMigrationProbes_CoverEveryMigration(t *testing.T) {
	names, err := migrationNames()
	if err != nil {
		t.Fatalf("listing embedded migrations: %v", err)
	}

	for _, name := range names {
		probe, ok := migrationProbes[name]
		if !ok {
			t.Errorf("%s has no entry in migrationProbes", name)
			continue
		}
		if strings.TrimSpace(probe) == "" {
			t.Errorf("%s has an empty probe", name)
		}
		// `::regclass` raises on a database that does not have the table;
		// to_regclass returns NULL. A probe that raises fails the boot.
		if strings.Contains(probe, "::regclass") {
			t.Errorf("%s probe uses ::regclass, which raises when the table is absent; use to_regclass", name)
		}
	}

	for name := range migrationProbes {
		if !slices.Contains(names, name) {
			t.Errorf("migrationProbes has an entry for %q, which is not an embedded migration", name)
		}
	}
}

// expectBookkeeping sets up the two calls MigrateTranscription always makes
// before it looks at any individual migration: create the table, read it.
func expectBookkeeping(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS schema_migrations_go")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT name FROM schema_migrations_go")).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
}

// This is the production case the probes exist for: the schema was applied by
// hand with `psql -f` long before the runner embedded these files, so the
// bookkeeping table is empty while every artefact is already there. Nothing may
// run — 002 alone would delete every row in chunks, transcripts and videos.
func TestMigrateTranscription_AdoptsAMigrationWhoseEffectIsAlreadyPresent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("opening sqlmock: %v", err)
	}
	defer db.Close()

	names, err := migrationNames()
	if err != nil {
		t.Fatalf("listing embedded migrations: %v", err)
	}

	expectBookkeeping(mock)
	for range names {
		mock.ExpectQuery("SELECT").
			WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(true))
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations_go")).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	// No ExpectBegin anywhere: an adopted migration is never executed, so it
	// never opens a transaction. sqlmock fails the test if one is opened.

	log := &fakeLogger{}
	if err := MigrateTranscription(context.Background(), db, log); err != nil {
		t.Fatalf("MigrateTranscription: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
	if !log.said("Adopted transcription migration") {
		t.Errorf("adoption was not logged; the boot log is the only record that it happened: %v", log.infos)
	}
}

// The fresh-database case: no artefact is present, so every file runs, each in
// its own transaction, with the shared-memory settings applied first.
func TestMigrateTranscription_AppliesWhenNothingIsPresent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("opening sqlmock: %v", err)
	}
	defer db.Close()

	names, err := migrationNames()
	if err != nil {
		t.Fatalf("listing embedded migrations: %v", err)
	}

	expectBookkeeping(mock)
	for range names {
		mock.ExpectQuery("SELECT").
			WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("SET LOCAL max_parallel_maintenance_workers")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations_go")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}

	log := &fakeLogger{}
	if err := MigrateTranscription(context.Background(), db, log); err != nil {
		t.Fatalf("MigrateTranscription: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// A probe that cannot answer must not be read as "not applied" — that would
// rerun a destructive migration on the one database where the probe mattered.
func TestMigrateTranscription_FailsWhenAProbeErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("opening sqlmock: %v", err)
	}
	defer db.Close()

	expectBookkeeping(mock)
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("permission denied for schema pg_catalog"))

	log := &fakeLogger{}
	err = MigrateTranscription(context.Background(), db, log)
	if err == nil {
		t.Fatal("boot continued after a probe failed")
	}
	if !strings.Contains(err.Error(), "already applied") {
		t.Errorf("error does not say the probe is what failed: %v", err)
	}
}

func excerpt(body string, idx int) string {
	start := max(idx-40, 0)
	end := min(idx+40, len(body))
	return body[start:end]
}
