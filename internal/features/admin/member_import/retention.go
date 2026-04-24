package member_import

import (
	"context"
	"database/sql"
	"time"
)

// StartupReset marks any UserImport rows stuck in "processing" for more than
// 5 minutes as failed. Call once during application bootstrap, before the
// HTTP server starts accepting new imports, to clean up records orphaned by
// a prior crash or restart.
func StartupReset(db *sql.DB, log interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}) {
	const q = `
		UPDATE "UserImport"
		SET status = 'failed',
		    "errorMessage" = 'server restarted',
		    "finishedAt" = NOW()
		WHERE status = 'processing'
		  AND "startedAt" < NOW() - INTERVAL '5 minutes'
	`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := db.ExecContext(ctx, q)
	if err != nil {
		log.Error("import.startup_reset_failed", "error", err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	log.Info("import.startup_reset", "rows_failed", affected)
}

// StartRetentionJob kicks off a ticker-driven cleanup that deletes
// UserImportRow rows older than 90 days. UserImport headers are kept
// forever.
//
// The goroutine runs until ctx is cancelled, which will happen when the
// server shuts down (pass the signal-aware context from main.go).
func StartRetentionJob(ctx context.Context, db *sql.DB, log interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}) {
	go func() {
		// Run once at startup so we don't wait a full 24h on a fresh deploy.
		runRetention(ctx, db, log)

		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runRetention(ctx, db, log)
			}
		}
	}()
}

// runRetention deletes in 10k-row chunks until a sweep returns 0. Chunking
// keeps the transaction short and avoids holding locks during large
// deletions.
func runRetention(parent context.Context, db *sql.DB, log interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}) {
	const q = `
		DELETE FROM "UserImportRow"
		WHERE id IN (
			SELECT id FROM "UserImportRow"
			WHERE "createdAt" < NOW() - INTERVAL '90 days'
			LIMIT 10000
		)
	`
	for {
		ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
		res, err := db.ExecContext(ctx, q)
		cancel()

		if err != nil {
			log.Error("import.retention_failed", "error", err.Error())
			return
		}
		n, _ := res.RowsAffected()
		log.Info("import.retention_sweep", "rows_deleted", n)
		if n == 0 {
			return
		}
		if parent.Err() != nil {
			return
		}
	}
}
