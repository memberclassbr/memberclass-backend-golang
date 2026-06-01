package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// Backfill runs the one-shot historical migration:
//  1. Populate Read.readAt and Read.tenantId for legacy rows.
//  2. Migrate UserEvent rows in the [from, to) range into typed event tables.
//  3. Run RunForUTCInstant per day.
//  4. Run RunForMonth for each fully-closed month.
//
// from/to are YYYY-MM strings inclusive of from-month and exclusive of to-month+1.
// When tenantId is non-empty, the run is scoped to that single tenant and never
// deletes raw data (history is kept until the operator validates the dashboard).
// BackfillOpts carries the tunables shared by single-tenant and all-tenants runs.
type BackfillOpts struct {
	FromMonth     string        // YYYY-MM inclusive
	ToMonth       string        // YYYY-MM inclusive (range is [from, to+1month))
	SkipUserEvent bool          // skip Read fixups + event migration; only roll up
	Reset         bool          // delete this tenant's backfill-derived rows first
	ChunkSize     int           // rows per set-based chunk / batched delete
	Sleep         time.Duration // pause between chunks
}

func Backfill(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string, o BackfillOpts) error {
	from, err := parseMonth(o.FromMonth)
	if err != nil {
		return fmt.Errorf("parse from: %w", err)
	}
	toBase, err := parseMonth(o.ToMonth)
	if err != nil {
		return fmt.Errorf("parse to: %w", err)
	}
	to := toBase.AddDate(0, 1, 0)

	logger.Info("analytics backfill start", "from", from.String(), "to", to.String(), "tenantId", tenantId, "skipUserEvent", o.SkipUserEvent, "reset", o.Reset)

	if o.Reset {
		if err := resetTenantData(ctx, db, logger, tenantId, o.ChunkSize); err != nil {
			return fmt.Errorf("reset tenant data: %w", err)
		}
	}

	if !o.SkipUserEvent {
		// Step 1: legacy Read fixups.
		if err := backfillLegacyRead(ctx, db, logger, tenantId); err != nil {
			return fmt.Errorf("legacy Read backfill: %w", err)
		}

		// Step 2: distinct types report (operator confirms mapping before scale migration).
		if err := printDistinctTypes(ctx, db, logger, tenantId); err != nil {
			return fmt.Errorf("distinct types: %w", err)
		}

		// Step 3: UserEvent migration — set-based INSERT...SELECT with keyset
		// pagination (one pass over [from,to) per type, not one INSERT per row).
		if err := migrateUserEvents(ctx, db, logger, tenantId, from, to, o.ChunkSize, o.Sleep); err != nil {
			return fmt.Errorf("UserEvent migration: %w", err)
		}

		// Step 3b: CommentEvent/CommunityPostEvent from primary entities. Runs
		// BEFORE the rollups below so daily/monthly counts include comments/posts.
		if err := BackfillExtras(ctx, db, logger, tenantId); err != nil {
			logger.Error("backfill-extras failed", "tenantId", tenantId, "err", err.Error())
		}
	} else {
		logger.Info("skipping Read backfill + UserEvent migration (--skipUserEvent)")
	}

	// Step 4: daily rollup.
	// Single tenant → one transaction covering the whole [from, to) range
	// (~1000x faster than the per-day loop for multi-year backfills).
	// All tenants → fall back to the per-day loop using the existing job entry point.
	daily := NewDailyRollupJob(db, logger)
	if tenantId != "" {
		logger.Info("daily rollup step start", "tenantId", tenantId, "from", from.String(), "to", to.String())
		if derr := daily.RunForRangeForTenant(ctx, tenantId, from, to); derr != nil {
			logger.Error("daily rollup failed", "err", derr.Error())
		}
	} else {
		for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
			if derr := daily.RunForUTCInstant(ctx, d.Add(12*time.Hour)); derr != nil {
				logger.Error("daily rollup failed", "day", d.String(), "err", derr.Error())
			}
		}
	}

	// Step 5: monthly rollup per closed month.
	monthly := NewMonthlyRollupJob(db, logger)
	now := time.Now().UTC()
	for m := from; m.Before(to); m = m.AddDate(0, 1, 0) {
		end := m.AddDate(0, 1, 0)
		if !end.After(now) {
			var merr error
			if tenantId != "" {
				merr = monthly.RunForMonthForTenant(ctx, m, tenantId)
			} else {
				merr = monthly.RunForMonth(ctx, m)
			}
			if merr != nil {
				logger.Error("monthly rollup failed", "month", m.String(), "err", merr.Error())
			}
		}
	}

	logger.Info("analytics backfill done")
	return nil
}

func parseMonth(s string) (time.Time, error) {
	return time.Parse("2006-01", s)
}

// readBackfillBatchSize keeps each UPDATE's transaction below CockroachDB's
// lock-tracking memory budget (~1MB). 5k row updates run comfortably under that.
const readBackfillBatchSize = 5_000

func backfillLegacyRead(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string) error {
	// Step 1: readAt = createdAt for completed reads where readAt IS NULL.
	// Tenant scope is applied via the lesson chain because Read.tenantId may still be NULL.
	if err := batchUpdate(ctx, logger, "Read.readAt", func() (sql.Result, error) {
		if tenantId == "" {
			return db.ExecContext(ctx, `
				UPDATE "Read" SET "readAt" = "createdAt"
				WHERE "id" IN (
					SELECT "id" FROM "Read"
					WHERE "read" = true AND "readAt" IS NULL
					LIMIT $1
				)
			`, readBackfillBatchSize)
		}
		return db.ExecContext(ctx, `
			UPDATE "Read" SET "readAt" = "createdAt"
			WHERE "id" IN (
				SELECT r."id"
				FROM "Read" r, "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
				WHERE r."read" = true AND r."readAt" IS NULL
				  AND r."lessonId" IS NOT NULL
				  AND l."id" = r."lessonId"
				  AND m."id" = l."moduleId"
				  AND s."id" = m."sectionId"
				  AND c."id" = s."courseId"
				  AND v."id" = c."vitrineId"
				  AND v."tenantId" = $1
				LIMIT $2
			)
		`, tenantId, readBackfillBatchSize)
	}); err != nil {
		return err
	}

	// Step 2: tenantId = v.tenantId via lesson→module→section→course→vitrine chain.
	// Uses a correlated subquery so each batched UPDATE resolves the chain per row.
	return batchUpdate(ctx, logger, "Read.tenantId", func() (sql.Result, error) {
		if tenantId == "" {
			return db.ExecContext(ctx, `
				UPDATE "Read" SET "tenantId" = (
					SELECT v."tenantId"
					FROM "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
					WHERE l."id" = "Read"."lessonId"
					  AND m."id" = l."moduleId"
					  AND s."id" = m."sectionId"
					  AND c."id" = s."courseId"
					  AND v."id" = c."vitrineId"
					LIMIT 1
				)
				WHERE "id" IN (
					SELECT r."id"
					FROM "Read" r, "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
					WHERE r."tenantId" IS NULL
					  AND r."lessonId" IS NOT NULL
					  AND l."id" = r."lessonId"
					  AND m."id" = l."moduleId"
					  AND s."id" = m."sectionId"
					  AND c."id" = s."courseId"
					  AND v."id" = c."vitrineId"
					LIMIT $1
				)
			`, readBackfillBatchSize)
		}
		return db.ExecContext(ctx, `
			UPDATE "Read" SET "tenantId" = $1
			WHERE "id" IN (
				SELECT r."id"
				FROM "Read" r, "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
				WHERE r."tenantId" IS NULL
				  AND r."lessonId" IS NOT NULL
				  AND l."id" = r."lessonId"
				  AND m."id" = l."moduleId"
				  AND s."id" = m."sectionId"
				  AND c."id" = s."courseId"
				  AND v."id" = c."vitrineId"
				  AND v."tenantId" = $1
				LIMIT $2
			)
		`, tenantId, readBackfillBatchSize)
	})
}

// batchUpdate loops the given UPDATE until RowsAffected = 0 to keep each transaction
// small. Logs cumulative progress every batch.
func batchUpdate(ctx context.Context, logger ports.Logger, label string, run func() (sql.Result, error)) error {
	var total int64
	for {
		res, err := run()
		if err != nil {
			return fmt.Errorf("%s batch: %w", label, err)
		}
		n, raErr := res.RowsAffected()
		if raErr != nil {
			return fmt.Errorf("%s RowsAffected: %w", label, raErr)
		}
		total += n
		logger.Info(label+" batch", "rows", n, "total", total)
		if n == 0 {
			break
		}
	}
	logger.Info(label+" backfilled", "rows", total)
	return nil
}

func printDistinctTypes(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string) error {
	var (
		rows *sql.Rows
		err  error
	)
	if tenantId == "" {
		rows, err = db.QueryContext(ctx, `
			SELECT "type", COUNT(*) FROM "UserEvent" GROUP BY "type" ORDER BY 2 DESC LIMIT 200
		`)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT "type", COUNT(*) FROM "UserEvent"
			WHERE "usersOnTenantsTenantId" = $1
			GROUP BY "type" ORDER BY 2 DESC LIMIT 200
		`, tenantId)
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int64
		if err := rows.Scan(&t, &n); err != nil {
			return err
		}
		logger.Info("UserEvent type", "type", t, "count", n)
	}
	return rows.Err()
}

// eventMigration maps a set of legacy UserEvent.type values to a typed event
// table. The destination row reuses UserEvent.id so re-runs are idempotent via
// ON CONFLICT ("id") DO NOTHING.
type eventMigration struct {
	label        string
	destTable    string
	destCols     string // INSERT column list
	selectExpr   string // SELECT expressions from "UserEvent" ue
	typeIn       string // SQL literal list for type IN (...)
	requireWhere bool   // lesson/exam need a non-empty whereEvent
}

// userEventMigrations is a hardcoded allowlist — the SQL fragments below are
// constants, never user input.
var userEventMigrations = []eventMigration{
	{
		label:      "LoginEvent",
		destTable:  "LoginEvent",
		destCols:   `"id","tenantId","userId","createdAt"`,
		selectExpr: `ue.id, ue."usersOnTenantsTenantId", ue."usersOnTenantsUserId", ue."createdAt"`,
		typeIn:     `'login','user-login'`,
	},
	{
		label:        "LessonAccessEvent",
		destTable:    "LessonAccessEvent",
		destCols:     `"id","tenantId","userId","lessonId","createdAt"`,
		selectExpr:   `ue.id, ue."usersOnTenantsTenantId", ue."usersOnTenantsUserId", ue."whereEvent", ue."createdAt"`,
		typeIn:       `'lesson_viewed'`,
		requireWhere: true,
	},
	{
		label:        "ExamCompletionEvent",
		destTable:    "ExamCompletionEvent",
		destCols:     `"id","tenantId","userId","examId","passed","createdAt"`,
		selectExpr:   `ue.id, ue."usersOnTenantsTenantId", ue."usersOnTenantsUserId", ue."whereEvent", false, ue."createdAt"`,
		typeIn:       `'exam','exam-completed','exam_auto_register'`,
		requireWhere: true,
	},
}

const defaultBackfillChunk = 5_000

// migrateUserEvents copies a tenant's UserEvent rows in [from, to) into the
// typed event tables using set-based INSERT...SELECT with keyset (id > cursor)
// pagination. Replaces the old per-row INSERT + OFFSET loop: O(n) instead of
// O(n^2), and ~one statement per chunk instead of one per row.
func migrateUserEvents(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string, from, to time.Time, chunkSize int, sleep time.Duration) error {
	if chunkSize <= 0 {
		chunkSize = defaultBackfillChunk
	}
	for _, m := range userEventMigrations {
		whereExtra := ""
		if m.requireWhere {
			whereExtra = `AND ue."whereEvent" <> ''`
		}
		// $1 tenantId, $2 from, $3 to, $4 cursor, $5 chunkSize. SQL fragments are
		// constants from the allowlist above — no user input is interpolated.
		query := fmt.Sprintf(`
WITH chunk AS (
  SELECT ue.id FROM "UserEvent" ue
  WHERE ue."usersOnTenantsTenantId" = $1
    AND ue."usersOnTenantsUserId" IS NOT NULL
    AND ue."type" IN (%s)
    %s
    AND ue."createdAt" >= $2 AND ue."createdAt" < $3
    AND ue.id > $4
  ORDER BY ue.id LIMIT $5
),
ins AS (
  INSERT INTO %q (%s)
  SELECT %s FROM "UserEvent" ue WHERE ue.id IN (SELECT id FROM chunk)
  ON CONFLICT ("id") DO NOTHING
  RETURNING 1
)
SELECT (SELECT MAX(id) FROM chunk), (SELECT COUNT(*) FROM ins)
`, m.typeIn, whereExtra, m.destTable, m.destCols, m.selectExpr)

		cursor := ""
		var total int64
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			var maxId sql.NullString
			var inserted int64
			if err := db.QueryRowContext(ctx, query, tenantId, from, to, cursor, chunkSize).Scan(&maxId, &inserted); err != nil {
				return fmt.Errorf("%s migrate (tenant %s): %w", m.label, tenantId, err)
			}
			total += inserted
			logger.Info("UserEvent migrate batch", "dest", m.label, "tenantId", tenantId, "inserted", inserted, "total", total, "lastId", maxId.String)
			if !maxId.Valid {
				break
			}
			cursor = maxId.String
			if sleep > 0 {
				time.Sleep(sleep)
			}
		}
		logger.Info("UserEvent migrate done", "dest", m.label, "tenantId", tenantId, "total", total)
	}
	return nil
}

// backfillResetTables are the analytics tables fully rebuilt by a backfill, so
// they're safe to delete before a re-run. Deliberately EXCLUDES LessonWatchEvent
// (live-only heartbeat, not reconstructable), Read and CourseProgress.
var backfillResetTables = []string{
	"LoginEvent",
	"LessonAccessEvent",
	"ExamCompletionEvent",
	"CommentEvent",
	"CommunityPostEvent",
	"TenantDailyStats",
	"TenantDailyUserActivity",
	"StudentMonthlyStats",
}

// resetTenantData deletes a tenant's backfill-derived rows in batches (to stay
// under CockroachDB's lock budget) so a re-run re-inserts from scratch. Table
// names are a hardcoded allowlist; tenantId is bound.
func resetTenantData(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = defaultBackfillChunk
	}
	for _, t := range backfillResetTables {
		q := fmt.Sprintf(`DELETE FROM %q WHERE "tenantId" = $1 LIMIT $2`, t)
		var total int64
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			res, err := db.ExecContext(ctx, q, tenantId, chunkSize)
			if err != nil {
				return fmt.Errorf("reset %s: %w", t, err)
			}
			n, raErr := res.RowsAffected()
			if raErr != nil {
				return fmt.Errorf("reset %s RowsAffected: %w", t, raErr)
			}
			total += n
			if n == 0 {
				break
			}
		}
		if total > 0 {
			logger.Info("reset table", "table", t, "tenantId", tenantId, "deleted", total)
		}
	}
	return nil
}

// backfillTenantOrderBy maps an --order flag to a SQL ORDER BY clause. The
// returned text is a constant from this switch (never user input); tenant id is
// always the tiebreak so paging stays deterministic for equal counts.
func backfillTenantOrderBy(order string) string {
	switch order {
	case "size-asc": // smallest tenants first — validate the pipeline on cheap ones
		return `COUNT(*) ASC, "usersOnTenantsTenantId"`
	case "size-desc": // largest first
		return `COUNT(*) DESC, "usersOnTenantsTenantId"`
	default: // "id" — stable by tenant id
		return `"usersOnTenantsTenantId"`
	}
}

// ListTenantsWithData prints every tenant that has UserEvent data, in the SAME
// order BackfillAllTenants pages through (so the idx column lines up with
// --offset), with its event count and a running cumulative — use it to plan the
// migration waves.
func ListTenantsWithData(ctx context.Context, db *sql.DB, logger ports.Logger, order string) error {
	q := fmt.Sprintf(`
		SELECT "usersOnTenantsTenantId" AS t, COUNT(*) AS c
		FROM "UserEvent"
		WHERE "usersOnTenantsTenantId" IS NOT NULL
		GROUP BY "usersOnTenantsTenantId"
		ORDER BY %s`, backfillTenantOrderBy(order))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	idx := 0
	var cum int64
	for rows.Next() {
		var t string
		var c int64
		if err := rows.Scan(&t, &c); err != nil {
			return err
		}
		cum += c
		logger.Info("tenant with data", "idx", idx, "tenantId", t, "userEvents", c, "cumUserEvents", cum)
		idx++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	logger.Info("tenants with data summary", "order", order, "tenants", idx, "userEvents", cum)
	return nil
}

// listBackfillTenants returns tenant ids that have UserEvent data to migrate,
// in the requested order so --offset/--limit page through them stably.
func listBackfillTenants(ctx context.Context, db *sql.DB, order string) ([]string, error) {
	q := fmt.Sprintf(`
		SELECT "usersOnTenantsTenantId"
		FROM "UserEvent"
		WHERE "usersOnTenantsTenantId" IS NOT NULL
		GROUP BY "usersOnTenantsTenantId"
		ORDER BY %s`, backfillTenantOrderBy(order))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 256)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// BackfillAllTenants runs the per-tenant Backfill across tenants that have data,
// with bounded concurrency and per-chunk pacing. offset/limit page through the
// (stably ordered) tenant list so the migration can run in small, observable
// waves; the log prints the next offset to use. Each tenant is independent and
// idempotent, so the run honors ctx between chunks and can be stopped/resumed.
func BackfillAllTenants(ctx context.Context, db *sql.DB, logger ports.Logger, o BackfillOpts, concurrency, offset, limit int, order string) error {
	all, err := listBackfillTenants(ctx, db, order)
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}
	total := len(all)

	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	tenants := all[offset:]
	if limit > 0 && limit < len(tenants) {
		tenants = tenants[:limit]
	}

	if concurrency < 1 {
		concurrency = 1
	}
	logger.Info("backfill all-tenants start",
		"tenantsWithData", total, "order", order, "offset", offset, "processing", len(tenants),
		"concurrency", concurrency, "reset", o.Reset, "chunk", o.ChunkSize, "sleep", o.Sleep.String())

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var done, failed int

	for _, tenantId := range tenants {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(tenantId string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := Backfill(ctx, db, logger, tenantId, o); err != nil {
				logger.Error("tenant backfill failed", "tenantId", tenantId, "err", err.Error())
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			mu.Lock()
			done++
			d := done
			mu.Unlock()
			logger.Info("tenant backfill done", "tenantId", tenantId, "done", d, "wave", len(tenants))
		}(tenantId)
	}
	wg.Wait()
	logger.Info("backfill all-tenants finished",
		"done", done, "failed", failed, "wave", len(tenants), "nextOffset", offset+len(tenants), "totalWithData", total)
	if failed > 0 {
		return fmt.Errorf("%d/%d tenants failed (see logs); rerun is idempotent", failed, len(tenants))
	}
	return nil
}
