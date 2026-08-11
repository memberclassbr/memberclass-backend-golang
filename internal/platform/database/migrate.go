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

// MigrateTranscription applies any embedded migration the database has not
// seen, in filename order. It is safe to call on every boot: applied files are
// skipped, and each file runs inside its own transaction so a failure leaves
// nothing half-applied.
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

	pending := 0
	for _, name := range names {
		if applied[name] {
			continue
		}
		if err := applyMigration(ctx, db, name); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		log.Info("Applied transcription migration: " + name)
		pending++
	}

	if pending == 0 {
		log.Info("Transcription schema is up to date")
	}
	return nil
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

	if _, err := tx.ExecContext(ctx, string(statements)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations_go (name) VALUES ($1)`, name); err != nil {
		return err
	}

	return tx.Commit()
}
