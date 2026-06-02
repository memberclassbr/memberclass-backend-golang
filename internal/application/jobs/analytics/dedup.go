package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// Post-migration cleanup of the dual-write overlap.
//
// During the dual-write window each event produced TWO rows in a typed table:
// one written live (random id) and one created by the backfill from the legacy
// source (id reused from UserEvent/Comment/Post). The backfill rows are the
// authoritative copy; the live rows are the duplicates to drop.
//
// A typed row is a backfill row iff its id exists in the source table; a live
// duplicate otherwise. dedupSpec captures that source mapping per table.
//
// ORDER MATTERS: run Dedup (which uses the source to decide what to keep) and
// validate it BEFORE CleanupUserEvent (which deletes the source). Never run
// Dedup again after CleanupUserEvent — with the source gone, every row would
// look like a duplicate. Dedup has a guard that refuses when the source is
// empty, as a backstop.
type dedupSpec struct {
	dest     string // typed table to dedup
	source   string // table whose ids the backfill reused
	typePred string // predicate on the source for this mapping (unqualified); "" = all rows
}

var dedupSpecs = []dedupSpec{
	{dest: "LoginEvent", source: "UserEvent", typePred: `"type" IN ('login','user-login')`},
	{dest: "LessonAccessEvent", source: "UserEvent", typePred: `"type" = 'lesson_viewed'`},
	{dest: "ExamCompletionEvent", source: "UserEvent", typePred: `"type" IN ('exam','exam-completed','exam_auto_register')`},
	{dest: "CommentEvent", source: "Comment", typePred: ``},
	{dest: "CommunityPostEvent", source: "Post", typePred: ``},
}

// notExistsClause: the typed row's id is NOT in its source table → a live
// duplicate. "type" is unambiguous (the typed tables have no type column).
func (s dedupSpec) notExistsClause() string {
	extra := ""
	if s.typePred != "" {
		extra = " AND " + s.typePred
	}
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM %q WHERE %q."id" = %q."id"%s)`,
		s.source, s.source, s.dest, extra)
}

func (s dedupSpec) sourceCountSQL() string {
	where := "1=1"
	if s.typePred != "" {
		where = s.typePred
	}
	return fmt.Sprintf(`SELECT count(*) FROM %q WHERE %s`, s.source, where)
}

// Dedup removes the live dual-write duplicates from the typed event tables —
// rows created before the cutoff whose id is not in the source table. LessonWatchEvent
// is never touched (live-only, no backfill, no duplicates). Dry-run by default:
// counts what it WOULD delete; pass apply=false to only report.
func Dedup(ctx context.Context, db *sql.DB, logger ports.Logger, before time.Time, apply bool, chunkSize int, sleep time.Duration) error {
	if chunkSize <= 0 {
		chunkSize = defaultBackfillChunk
	}
	logger.Info("dedup start", "before", before.Format(time.RFC3339), "apply", apply, "chunk", chunkSize)

	var grandDeleted int64
	for _, s := range dedupSpecs {
		// Guard: an empty source means NOT EXISTS matches everything and would
		// wipe the table — refuse (the source was likely already cleaned up).
		var srcCount int64
		if err := db.QueryRowContext(ctx, s.sourceCountSQL()).Scan(&srcCount); err != nil {
			return fmt.Errorf("%s source count: %w", s.dest, err)
		}
		if srcCount == 0 {
			logger.Error("dedup SKIPPED: source empty (run dedup BEFORE cleanup-userevent)", "dest", s.dest, "source", s.source)
			continue
		}

		notExists := s.notExistsClause()

		var toDelete int64
		countSQL := fmt.Sprintf(`SELECT count(*) FROM %q WHERE "createdAt" < $1 AND %s`, s.dest, notExists)
		if err := db.QueryRowContext(ctx, countSQL, before).Scan(&toDelete); err != nil {
			return fmt.Errorf("%s count: %w", s.dest, err)
		}
		logger.Info("dedup plan", "dest", s.dest, "source", s.source, "sourceRows", srcCount, "duplicatesToDelete", toDelete)

		if !apply || toDelete == 0 {
			continue
		}

		delSQL := fmt.Sprintf(`DELETE FROM %q WHERE "createdAt" < $1 AND %s LIMIT $2`, s.dest, notExists)
		var deleted int64
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			res, err := db.ExecContext(ctx, delSQL, before, chunkSize)
			if err != nil {
				return fmt.Errorf("%s delete: %w", s.dest, err)
			}
			n, raErr := res.RowsAffected()
			if raErr != nil {
				return fmt.Errorf("%s RowsAffected: %w", s.dest, raErr)
			}
			deleted += n
			if n == 0 {
				break
			}
			if sleep > 0 {
				time.Sleep(sleep)
			}
		}
		grandDeleted += deleted
		logger.Info("dedup done", "dest", s.dest, "deleted", deleted)
	}

	logger.Info("dedup finished", "apply", apply, "totalDeleted", grandDeleted)
	if !apply {
		logger.Info("dedup was a DRY RUN — pass -apply to delete")
	}
	return nil
}

// CleanupUserEvent deletes the legacy UserEvent rows of the migrated types that
// have already been migrated (their id exists in the destination typed table).
// Run AFTER Dedup is validated. Dry-run by default; pass apply=true to delete.
// Comment/Post sources are left alone (they are primary entities, not legacy
// duplicates).
func CleanupUserEvent(ctx context.Context, db *sql.DB, logger ports.Logger, apply bool, chunkSize int, sleep time.Duration) error {
	if chunkSize <= 0 {
		chunkSize = defaultBackfillChunk
	}
	logger.Info("cleanup-userevent start", "apply", apply, "chunk", chunkSize)

	var grandDeleted int64
	for _, s := range dedupSpecs {
		if s.source != "UserEvent" {
			continue
		}
		// Only delete UserEvent rows confirmed migrated (id present in dest).
		migrated := fmt.Sprintf(`EXISTS (SELECT 1 FROM %q WHERE %q."id" = "UserEvent"."id")`, s.dest, s.dest)
		where := fmt.Sprintf(`%s AND %s`, s.typePred, migrated)

		var toDelete int64
		countSQL := fmt.Sprintf(`SELECT count(*) FROM "UserEvent" WHERE %s`, where)
		if err := db.QueryRowContext(ctx, countSQL).Scan(&toDelete); err != nil {
			return fmt.Errorf("%s userevent count: %w", s.dest, err)
		}
		logger.Info("cleanup-userevent plan", "type", s.typePred, "migratedRowsToDelete", toDelete)

		if !apply || toDelete == 0 {
			continue
		}

		delSQL := fmt.Sprintf(`DELETE FROM "UserEvent" WHERE %s LIMIT $1`, where)
		var deleted int64
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			res, err := db.ExecContext(ctx, delSQL, chunkSize)
			if err != nil {
				return fmt.Errorf("%s userevent delete: %w", s.dest, err)
			}
			n, raErr := res.RowsAffected()
			if raErr != nil {
				return fmt.Errorf("%s RowsAffected: %w", s.dest, raErr)
			}
			deleted += n
			if n == 0 {
				break
			}
			if sleep > 0 {
				time.Sleep(sleep)
			}
		}
		grandDeleted += deleted
		logger.Info("cleanup-userevent done", "type", s.typePred, "deleted", deleted)
	}

	logger.Info("cleanup-userevent finished", "apply", apply, "totalDeleted", grandDeleted)
	if !apply {
		logger.Info("cleanup-userevent was a DRY RUN — pass -apply to delete")
	}
	return nil
}
