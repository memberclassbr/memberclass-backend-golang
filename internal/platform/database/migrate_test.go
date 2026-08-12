package database

import (
	"regexp"
	"strings"
	"testing"
)

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

func excerpt(body string, idx int) string {
	start := max(idx-40, 0)
	end := min(idx+40, len(body))
	return body[start:end]
}
