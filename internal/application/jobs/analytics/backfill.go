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
	label             string
	destTable         string
	destCols          string // INSERT column list
	selectExpr        string // SELECT expressions from "UserEvent" ue
	typeIn            string // SQL literal list for type IN (...)
	requireIdentifier bool   // lesson/exam carry the lessonId/examId in UserEvent.identifier
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
		// lessonId is in UserEvent.identifier (whereEvent holds the page path).
		label:             "LessonAccessEvent",
		destTable:         "LessonAccessEvent",
		destCols:          `"id","tenantId","userId","lessonId","createdAt"`,
		selectExpr:        `ue.id, ue."usersOnTenantsTenantId", ue."usersOnTenantsUserId", ue."identifier", ue."createdAt"`,
		typeIn:            `'lesson_viewed'`,
		requireIdentifier: true,
	},
	{
		// examId is in UserEvent.identifier (whereEvent holds the answers JSON).
		label:             "ExamCompletionEvent",
		destTable:         "ExamCompletionEvent",
		destCols:          `"id","tenantId","userId","examId","passed","createdAt"`,
		selectExpr:        `ue.id, ue."usersOnTenantsTenantId", ue."usersOnTenantsUserId", ue."identifier", false, ue."createdAt"`,
		typeIn:            `'exam','exam-completed','exam_auto_register'`,
		requireIdentifier: true,
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
		if m.requireIdentifier {
			whereExtra = `AND ue."identifier" IS NOT NULL AND ue."identifier" <> ''`
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

// pendingUserEventWhere matches handled-type UserEvent rows NOT yet migrated to
// their destination table (the destination reuses UserEvent.id). It's how we
// count remaining work and skip tenants already fully migrated — a tenant drops
// off the list/queue once its events are in the typed tables. A --reset deletes
// the destination rows, so the tenant correctly reappears as pending.
// The surrounding query must alias UserEvent as `ue`.
const pendingUserEventWhere = `ue."usersOnTenantsTenantId" IS NOT NULL AND (
		(ue."type" IN ('login','user-login')
		   AND NOT EXISTS (SELECT 1 FROM "LoginEvent" d WHERE d."id" = ue."id"))
		OR (ue."type" = 'lesson_viewed' AND ue."whereEvent" <> ''
		   AND NOT EXISTS (SELECT 1 FROM "LessonAccessEvent" d WHERE d."id" = ue."id"))
		OR (ue."type" IN ('exam','exam-completed','exam_auto_register') AND ue."whereEvent" <> ''
		   AND NOT EXISTS (SELECT 1 FROM "ExamCompletionEvent" d WHERE d."id" = ue."id"))
	)`

// backfillTenantOrderBy maps an --order flag to an ORDER BY over the pending
// query's aliases (c = pending event count, t = tenant id). Constant from this
// switch (never user input); tenant id is always the tiebreak.
func backfillTenantOrderBy(order string) string {
	switch order {
	case "size-asc": // fewest pending events first — validate on cheap tenants
		return `c ASC, t`
	case "size-desc": // most pending first
		return `c DESC, t`
	default: // "id" — stable by tenant id
		return `t`
	}
}

// ListTenantsWithData prints tenants that still have data to migrate (handled
// UserEvent rows not yet in the typed tables), in the order the backfill
// processes them, with the pending count + running cumulative. Fully-migrated
// tenants are omitted, so the list shrinks as you go.
func ListTenantsWithData(ctx context.Context, db *sql.DB, logger ports.Logger, order string) error {
	q := fmt.Sprintf(`
		SELECT ue."usersOnTenantsTenantId" AS t, COUNT(*) AS c
		FROM "UserEvent" ue
		WHERE %s
		GROUP BY ue."usersOnTenantsTenantId"
		HAVING COUNT(*) > 0
		ORDER BY %s`, pendingUserEventWhere, backfillTenantOrderBy(order))
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
		logger.Info("tenant pending", "idx", idx, "tenantId", t, "pendingEvents", c, "cumPending", cum)
		idx++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Legacy Read fixup still pending (completed reads missing readAt). Not
	// attributed per tenant here — Read.tenantId is set BY the backfill itself.
	var readPending int64
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM "Read" WHERE "read" = true AND "readAt" IS NULL`).Scan(&readPending); err != nil {
		return err
	}

	logger.Info("pending summary", "order", order,
		"tenantsPending", idx, "pendingEvents", cum, "readsMissingReadAt", readPending)
	return nil
}

// listBackfillTenants returns tenants that still have handled UserEvent rows not
// yet migrated, in the requested order. Fully-migrated tenants are excluded, so
// each run picks up the next pending tenants (the list shrinks — use --offset=0
// and just re-run until it's empty).
func listBackfillTenants(ctx context.Context, db *sql.DB, order string) ([]string, error) {
	q := fmt.Sprintf(`
		SELECT ue."usersOnTenantsTenantId" AS t, COUNT(*) AS c
		FROM "UserEvent" ue
		WHERE %s
		GROUP BY ue."usersOnTenantsTenantId"
		HAVING COUNT(*) > 0
		ORDER BY %s`, pendingUserEventWhere, backfillTenantOrderBy(order))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 256)
	for rows.Next() {
		var t string
		var c int64
		if err := rows.Scan(&t, &c); err != nil {
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
