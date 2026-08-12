package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// transcriptionMigrations is the schema for the pgvector database, embedded in
// the binary so a fresh deployment brings its own schema. The tenant database
// is not covered here: that schema is owned by Prisma in the frontend
// repository and applied from there.
//
//go:embed migrations/transcription/*.sql
var transcriptionMigrations embed.FS

// sqlMigrationsTable records which files have been applied. Its name is
// deliberately distinct from anything Prisma creates.
const sqlMigrationsTable = `
	CREATE TABLE IF NOT EXISTS schema_migrations_go (
		name        TEXT PRIMARY KEY,
		applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)
`

// sqlMigrationSessionSettings bounds what a migration may ask of shared memory,
// for the transaction it is applied in.
//
// Postgres in a container gets Docker's default 64MB /dev/shm, and pgvector
// sizes the dynamic shared memory segment for a parallel HNSW build from
// maintenance_work_mem. On a managed instance that lands near 61MB, which does
// not fit, and the index build dies at boot:
//
//	migration 002_embedding_1536.sql: pq: could not resize shared memory
//	segment "/PostgreSQL.2625290996" to 64001056 bytes: No space left on device
//
// Zero maintenance workers keeps the build off dynamic shared memory entirely —
// pgvector only reaches for a segment on the parallel path — and the lowered
// maintenance_work_mem bounds the request on any path that still takes one.
// Both cost build speed on a large index and buy a boot that completes, which
// is the right trade for a migration: an index built slowly is an index, and a
// deployment that will not start is nothing.
const sqlMigrationSessionSettings = `
	SET LOCAL max_parallel_maintenance_workers = 0;
	SET LOCAL maintenance_work_mem = '32MB';
`

// migrationProbes answers, for one migration, the question the bookkeeping
// table cannot: is this file's effect already in the database?
//
// It exists because `schema_migrations_go` is younger than the schema it
// tracks. These migrations were run by hand with `psql -f` before the runner
// embedded them, so a database can be fully migrated and still have an empty
// bookkeeping table — the runner then sees every file as pending and tries to
// apply it a second time. On an empty database that is merely wasteful. On one
// with data it is destructive: 002 deletes every row in chunks, transcripts and
// videos, and rebuilding an HNSW index over real data is what asked for a
// shared memory segment bigger than the container's /dev/shm.
//
// So each probe looks for the artefact the migration leaves behind, and a
// pending migration whose probe says "already there" is recorded as applied
// without being run. Adoption happens once per database; from then on the
// bookkeeping table answers on its own.
//
// Two rules for a probe:
//
//   - It must return exactly one boolean row and it must never raise. A fresh
//     database has none of these tables, so `to_regclass` (NULL when absent)
//     rather than `::regclass` (an error), and no bare reference to a table
//     that may not exist. A probe that errors fails the boot.
//   - It must be specific to that migration's artefact. A false positive
//     records a migration as applied that never ran, and nothing will run it
//     later.
//
// 000 has no artefact of its own — it deletes duplicate rows — so it borrows
// 001's unique index. That index cannot exist unless the deduplication it
// depends on succeeded, which is exactly what 000 does.
var migrationProbes = map[string]string{
	"000_dedupe_videos.sql": `
		SELECT to_regclass('public.videos_unique_tenant_source') IS NOT NULL`,

	"001_pgvector_index.sql": `
		SELECT to_regclass('public.videos_unique_tenant_source') IS NOT NULL
		   AND to_regclass('public.chunks_embedding_hnsw_cosine') IS NOT NULL`,

	"002_embedding_1536.sql": `
		SELECT EXISTS (
			SELECT 1
			  FROM pg_attribute a
			 WHERE a.attrelid = to_regclass('public.chunks')
			   AND a.attname  = 'embedding'
			   AND NOT a.attisdropped
			   AND format_type(a.atttypid, a.atttypmod) = 'vector(1536)'
		)`,

	"003_panda_source_type.sql": `
		SELECT EXISTS (
			SELECT 1
			  FROM pg_enum e
			  JOIN pg_type t ON t.oid = e.enumtypid
			 WHERE t.typname   = 'video_source_type'
			   AND e.enumlabel = 'PANDA_VIDEO'
		)`,
}

// MigrateTranscription applies any embedded migration the database has not
// seen, in filename order. It is safe to call on every boot: applied files are
// skipped, a file whose effect is already present is adopted rather than rerun
// (see migrationProbes), and anything that does run does so inside its own
// transaction, so a failure leaves nothing half-applied.
func MigrateTranscription(ctx context.Context, db *sql.DB, log logger.Logger) error {
	if db == nil {
		return nil
	}

	if _, err := db.ExecContext(ctx, sqlMigrationsTable); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}

	changed := 0
	for _, name := range names {
		if applied[name] {
			continue
		}

		present, err := alreadyInPlace(ctx, db, name)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if present {
			if err := recordMigration(ctx, db, name); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			log.Info("Adopted transcription migration, its effect was already in the database: " + name)
			changed++
			continue
		}

		if err := applyMigration(ctx, db, name); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		log.Info("Applied transcription migration: " + name)
		changed++
	}

	if changed == 0 {
		log.Info("Transcription schema is up to date")
	}
	return nil
}

// alreadyInPlace runs the migration's probe. A migration with no probe is never
// adopted — the safe default, since the cost of rerunning is a migration
// written to tolerate it, while the cost of a wrong adoption is a migration
// that never runs at all.
func alreadyInPlace(ctx context.Context, db *sql.DB, name string) (bool, error) {
	probe, ok := migrationProbes[name]
	if !ok {
		return false, nil
	}

	var present bool
	if err := db.QueryRowContext(ctx, probe).Scan(&present); err != nil {
		return false, fmt.Errorf("probe whether it is already applied: %w", err)
	}
	return present, nil
}

// recordMigration marks a migration as applied. ON CONFLICT DO NOTHING so two
// instances booting together cannot turn a race into a failed start.
func recordMigration(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, name string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO schema_migrations_go (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, name)
	return err
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations_go`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// migrationNames returns the embedded files sorted by name, which is what
// defines their order — hence the numeric prefixes.
func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(transcriptionMigrations, "migrations/transcription")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string) error {
	statements, err := transcriptionMigrations.ReadFile("migrations/transcription/" + name)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// SET LOCAL, so this is scoped to the migration's own transaction and no
	// connection goes back to the pool carrying it.
	if _, err := tx.ExecContext(ctx, sqlMigrationSessionSettings); err != nil {
		return fmt.Errorf("apply migration session settings: %w", err)
	}

	if _, err := tx.ExecContext(ctx, string(statements)); err != nil {
		return err
	}
	if err := recordMigration(ctx, tx, name); err != nil {
		return err
	}

	return tx.Commit()
}
