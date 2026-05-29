package analytics

import (
	"context"
	"database/sql"
	"fmt"
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
func Backfill(ctx context.Context, db *sql.DB, logger ports.Logger, fromMonth, toMonth, tenantId string) error {
	from, err := parseMonth(fromMonth)
	if err != nil {
		return fmt.Errorf("parse from: %w", err)
	}
	toBase, err := parseMonth(toMonth)
	if err != nil {
		return fmt.Errorf("parse to: %w", err)
	}
	to := toBase.AddDate(0, 1, 0)

	logger.Info("analytics backfill start", "from", from.String(), "to", to.String(), "tenantId", tenantId)

	// Step 1: legacy Read fixups.
	if err := backfillLegacyRead(ctx, db, logger, tenantId); err != nil {
		return fmt.Errorf("legacy Read backfill: %w", err)
	}

	// Step 2: distinct types report (operator confirms mapping before scale migration).
	if err := printDistinctTypes(ctx, db, logger, tenantId); err != nil {
		return fmt.Errorf("distinct types: %w", err)
	}

	// Step 3: per-month UserEvent migration.
	for m := from; m.Before(to); m = m.AddDate(0, 1, 0) {
		end := m.AddDate(0, 1, 0)
		if err := migrateUserEventsRange(ctx, db, logger, m, end, tenantId); err != nil {
			logger.Error("migrate range failed", "month", m.String(), "err", err.Error())
		}
	}

	// Step 4: daily rollup per UTC day in range.
	daily := NewDailyRollupJob(db, logger)
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		var derr error
		if tenantId != "" {
			derr = daily.RunForUTCInstantForTenant(ctx, d.Add(12*time.Hour), tenantId)
		} else {
			derr = daily.RunForUTCInstant(ctx, d.Add(12*time.Hour))
		}
		if derr != nil {
			logger.Error("daily rollup failed", "day", d.String(), "err", derr.Error())
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

func backfillLegacyRead(ctx context.Context, db *sql.DB, logger ports.Logger, tenantId string) error {
	// Populate readAt for completed reads where it's NULL.
	// Tenant scope is applied via the lesson chain because Read.tenantId itself may still be NULL.
	var (
		res sql.Result
		err error
	)
	if tenantId == "" {
		res, err = db.ExecContext(ctx, `
			UPDATE "Read" SET "readAt" = "createdAt"
			WHERE "read" = true AND "readAt" IS NULL
		`)
	} else {
		res, err = db.ExecContext(ctx, `
			UPDATE "Read" r
			SET "readAt" = r."createdAt"
			FROM "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
			WHERE r."read" = true AND r."readAt" IS NULL
			  AND r."lessonId" IS NOT NULL
			  AND l."id" = r."lessonId"
			  AND m."id" = l."moduleId"
			  AND s."id" = m."sectionId"
			  AND c."id" = s."courseId"
			  AND v."id" = c."vitrineId"
			  AND v."tenantId" = $1
		`, tenantId)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	logger.Info("Read.readAt backfilled", "rows", n)

	// Populate tenantId via lesson→module→section→course→vitrine→tenant chain.
	if tenantId == "" {
		res, err = db.ExecContext(ctx, `
			UPDATE "Read" r
			SET "tenantId" = v."tenantId"
			FROM "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
			WHERE r."tenantId" IS NULL
			  AND r."lessonId" IS NOT NULL
			  AND l."id" = r."lessonId"
			  AND m."id" = l."moduleId"
			  AND s."id" = m."sectionId"
			  AND c."id" = s."courseId"
			  AND v."id" = c."vitrineId"
		`)
	} else {
		res, err = db.ExecContext(ctx, `
			UPDATE "Read" r
			SET "tenantId" = v."tenantId"
			FROM "Lesson" l, "Module" m, "Section" s, "Course" c, "Vitrine" v
			WHERE r."tenantId" IS NULL
			  AND r."lessonId" IS NOT NULL
			  AND l."id" = r."lessonId"
			  AND m."id" = l."moduleId"
			  AND s."id" = m."sectionId"
			  AND c."id" = s."courseId"
			  AND v."id" = c."vitrineId"
			  AND v."tenantId" = $1
		`, tenantId)
	}
	if err != nil {
		return err
	}
	n, _ = res.RowsAffected()
	logger.Info("Read.tenantId backfilled", "rows", n)
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

// migrateUserEventsRange copies UserEvent rows in [from, to) into typed event tables.
// Unmapped types are skipped and counted. Uses 50k-row chunks.
func migrateUserEventsRange(ctx context.Context, db *sql.DB, logger ports.Logger, from, to time.Time, tenantId string) error {
	const chunk = 50_000
	var offset int
	mapped := 0
	skipped := 0
	for {
		var (
			rows *sql.Rows
			err  error
		)
		if tenantId == "" {
			rows, err = db.QueryContext(ctx, `
				SELECT id, type, "whereEvent", "withEvent", value,
				       "usersOnTenantsTenantId", "usersOnTenantsUserId", "createdAt"
				FROM "UserEvent"
				WHERE "createdAt" >= $1 AND "createdAt" < $2
				ORDER BY "createdAt", id
				LIMIT $3 OFFSET $4
			`, from, to, chunk, offset)
		} else {
			rows, err = db.QueryContext(ctx, `
				SELECT id, type, "whereEvent", "withEvent", value,
				       "usersOnTenantsTenantId", "usersOnTenantsUserId", "createdAt"
				FROM "UserEvent"
				WHERE "createdAt" >= $1 AND "createdAt" < $2
				  AND "usersOnTenantsTenantId" = $5
				ORDER BY "createdAt", id
				LIMIT $3 OFFSET $4
			`, from, to, chunk, offset, tenantId)
		}
		if err != nil {
			return err
		}
		read := 0
		for rows.Next() {
			read++
			var (
				id, evType                                  string
				whereEvent, withEvent                       string
				value                                       sql.NullInt64
				tenantIdN, userIdN                          sql.NullString
				createdAt                                   time.Time
			)
			if err := rows.Scan(&id, &evType, &whereEvent, &withEvent, &value, &tenantIdN, &userIdN, &createdAt); err != nil {
				rows.Close()
				return err
			}
			if !tenantIdN.Valid || !userIdN.Valid {
				skipped++
				continue
			}
			if migrateRow(ctx, db, evType, whereEvent, withEvent, value, tenantIdN.String, userIdN.String, createdAt) {
				mapped++
			} else {
				skipped++
			}
		}
		rows.Close()
		if read < chunk {
			break
		}
		offset += chunk
	}
	logger.Info("UserEvent migration chunk done", "from", from.String(), "mapped", mapped, "skipped", skipped)
	return nil
}

func migrateRow(ctx context.Context, db *sql.DB, evType, whereEvent, withEvent string, _ sql.NullInt64, tenantId, userId string, createdAt time.Time) bool {
	switch evType {
	case "login", "user-login":
		_, _ = db.ExecContext(ctx, `
			INSERT INTO "LoginEvent" ("id","tenantId","userId","createdAt")
			VALUES (gen_random_uuid()::string, $1, $2, $3)
		`, tenantId, userId, createdAt)
		return true
	case "lesson_viewed":
		if whereEvent == "" {
			return false
		}
		_, _ = db.ExecContext(ctx, `
			INSERT INTO "LessonAccessEvent" ("id","tenantId","userId","lessonId","createdAt")
			VALUES (gen_random_uuid()::string, $1, $2, $3, $4)
		`, tenantId, userId, whereEvent, createdAt)
		return true
	case "exam", "exam-completed", "exam_auto_register":
		if whereEvent == "" {
			return false
		}
		_, _ = db.ExecContext(ctx, `
			INSERT INTO "ExamCompletionEvent" ("id","tenantId","userId","examId","passed","createdAt")
			VALUES (gen_random_uuid()::string, $1, $2, $3, false, $4)
		`, tenantId, userId, whereEvent, createdAt)
		return true
	default:
		return false
	}
}
