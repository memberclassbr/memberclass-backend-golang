package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// MonthlyRollupJob aggregates a closed month per (tenant, user) into
// StudentMonthlyStats and (when ANALYTICS_DELETE_ENABLED=true) deletes the
// raw events for that month. Cron: 0 0 9 1 * * UTC (day 1, 09:00).
type MonthlyRollupJob struct {
	db     *sql.DB
	logger ports.Logger
}

func NewMonthlyRollupJob(db *sql.DB, logger ports.Logger) *MonthlyRollupJob {
	return &MonthlyRollupJob{db: db, logger: logger}
}

func (j *MonthlyRollupJob) Name() string { return "analytics.monthly_rollup" }

func (j *MonthlyRollupJob) Execute(ctx context.Context) error {
	now := time.Now().UTC()
	firstThis := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	prev := firstThis.AddDate(0, -1, 0)
	return j.RunForMonth(ctx, prev)
}

func (j *MonthlyRollupJob) RunForMonth(ctx context.Context, month time.Time) error {
	next := month.AddDate(0, 1, 0)

	tenants, err := j.listTenantsWithActivity(ctx, month, next)
	if err != nil {
		return err
	}

	for _, tenantId := range tenants {
		if err := j.rollupTenantCounters(ctx, tenantId, month, next); err != nil {
			j.logger.Error("counters rollup failed", "tenantId", tenantId, "err", err.Error())
			continue
		}
		if err := j.rollupTenantDetails(ctx, tenantId, month, next); err != nil {
			j.logger.Error("details rollup failed", "tenantId", tenantId, "err", err.Error())
		}
	}

	if os.Getenv("ANALYTICS_DELETE_ENABLED") == "true" {
		return j.deleteRawMonth(ctx, month, next)
	}
	return nil
}

// RunForMonthForTenant is the single-tenant variant used by backfill. Never deletes
// raw data — backfill workflow keeps history intact.
func (j *MonthlyRollupJob) RunForMonthForTenant(ctx context.Context, month time.Time, tenantId string) error {
	next := month.AddDate(0, 1, 0)
	if err := j.rollupTenantCounters(ctx, tenantId, month, next); err != nil {
		return fmt.Errorf("counters: %w", err)
	}
	if err := j.rollupTenantDetails(ctx, tenantId, month, next); err != nil {
		return fmt.Errorf("details: %w", err)
	}
	return nil
}

func (j *MonthlyRollupJob) listTenantsWithActivity(ctx context.Context, from, to time.Time) ([]string, error) {
	rows, err := j.db.QueryContext(ctx, `
		SELECT DISTINCT "tenantId" FROM "TenantDailyUserActivity"
		WHERE "day" >= $1 AND "day" < $2
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 64)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// rollupTenantCounters: single SQL with pre-aggregated CTEs LEFT-JOINed against
// TenantDailyUserActivity. O(n) hash joins vs O(n²) correlated subqueries.
// $1 tenantId, $2 monthStart (Date for uda.day), $3 monthStart (Timestamp for raw.createdAt),
// $4 monthEnd (Timestamp), $5 monthEnd (Date).
func (j *MonthlyRollupJob) rollupTenantCounters(ctx context.Context, tenantId string, monthStart, monthEnd time.Time) error {
	_, err := j.db.ExecContext(ctx, `
WITH la AS (
  SELECT "userId", COUNT(*) AS cnt FROM "LessonAccessEvent"
  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4 GROUP BY "userId"
),
qz AS (
  SELECT "studentId" AS "userId", COUNT(*) AS cnt FROM "StudentQuiz"
  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
    AND "quizId" IS NOT NULL AND "score" IS NOT NULL AND "deletedAt" IS NULL
  GROUP BY "studentId"
),
ex AS (
  SELECT "userId",
         COUNT(*) AS cnt,
         COUNT(*) FILTER (WHERE "passed") AS passed_cnt
  FROM "ExamCompletionEvent"
  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4 GROUP BY "userId"
),
cm AS (
  SELECT "userId", COUNT(*) AS cnt FROM "CommentEvent"
  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4 GROUP BY "userId"
),
pt AS (
  SELECT "userId", COUNT(*) AS cnt FROM "CommunityPostEvent"
  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4 GROUP BY "userId"
),
rt AS (
  SELECT "userId", AVG("rating")::float AS avg FROM "Read"
  WHERE "tenantId" = $1 AND "rating" IS NOT NULL AND "createdAt" >= $3 AND "createdAt" < $4 GROUP BY "userId"
),
agg AS (
  SELECT
    uda."userId",
    COUNT(*) FILTER (WHERE uda."didLogin")::int AS login_days,
    COALESCE(SUM(uda."lessonReadCount"), 0)::int AS read_cnt,
    COALESCE(SUM(uda."watchSec"), 0)::int        AS watch_sec
  FROM "TenantDailyUserActivity" uda
  WHERE uda."tenantId" = $1 AND uda."day" >= $2 AND uda."day" < $5
  GROUP BY uda."userId"
)
INSERT INTO "StudentMonthlyStats" (
  "userId","tenantId","month",
  "loginDays","lessonAccessCount","lessonReadCount","watchSeconds",
  "quizCount","examCount","examPassedCount","commentCount","communityPostCount",
  "ratingAvg","details"
)
SELECT
  agg."userId", $1, $2,
  agg.login_days,
  COALESCE(la.cnt, 0),
  agg.read_cnt,
  agg.watch_sec,
  COALESCE(qz.cnt, 0),
  COALESCE(ex.cnt, 0),
  COALESCE(ex.passed_cnt, 0),
  COALESCE(cm.cnt, 0),
  COALESCE(pt.cnt, 0),
  rt.avg,
  '{}'::jsonb
FROM agg
LEFT JOIN la ON la."userId" = agg."userId"
LEFT JOIN qz ON qz."userId" = agg."userId"
LEFT JOIN ex ON ex."userId" = agg."userId"
LEFT JOIN cm ON cm."userId" = agg."userId"
LEFT JOIN pt ON pt."userId" = agg."userId"
LEFT JOIN rt ON rt."userId" = agg."userId"
ON CONFLICT ("userId","tenantId","month") DO UPDATE SET
  "loginDays"=EXCLUDED."loginDays", "lessonAccessCount"=EXCLUDED."lessonAccessCount",
  "lessonReadCount"=EXCLUDED."lessonReadCount", "watchSeconds"=EXCLUDED."watchSeconds",
  "quizCount"=EXCLUDED."quizCount", "examCount"=EXCLUDED."examCount",
  "examPassedCount"=EXCLUDED."examPassedCount", "commentCount"=EXCLUDED."commentCount",
  "communityPostCount"=EXCLUDED."communityPostCount", "ratingAvg"=EXCLUDED."ratingAvg"
  -- details NOT touched on conflict; rollupTenantDetails owns it
`,
		tenantId,   // $1
		monthStart, // $2
		monthStart, // $3
		monthEnd,   // $4
		monthEnd,   // $5
	)
	return err
}

func (j *MonthlyRollupJob) rollupTenantDetails(ctx context.Context, tenantId string, monthStart, monthEnd time.Time) error {
	rows, err := j.db.QueryContext(ctx, `
		SELECT "userId" FROM "StudentMonthlyStats"
		WHERE "tenantId" = $1 AND "month" = $2
	`, tenantId, monthStart)
	if err != nil {
		return err
	}
	defer rows.Close()
	userIds := make([]string, 0, 256)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return err
		}
		userIds = append(userIds, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, userId := range userIds {
		details, err := buildStudentDetailsJSON(ctx, j.db, tenantId, userId, monthStart, monthEnd)
		if err != nil {
			j.logger.Error("details build failed", "userId", userId, "err", err.Error())
			continue
		}
		if _, err := j.db.ExecContext(ctx, `
			UPDATE "StudentMonthlyStats" SET "details" = $1
			WHERE "userId" = $2 AND "tenantId" = $3 AND "month" = $4
		`, details, userId, tenantId, monthStart); err != nil {
			j.logger.Error("details update failed", "userId", userId, "err", err.Error())
		}
	}
	return nil
}

type lessonAgg struct {
	LessonId string    `json:"lessonId"`
	Seconds  int       `json:"seconds"`
	LastAt   time.Time `json:"lastAt"`
}
type examRow struct {
	ExamId string    `json:"examId"`
	Score  *int      `json:"score"`
	Passed bool      `json:"passed"`
	At     time.Time `json:"at"`
}
type quizRow struct {
	QuizId   string    `json:"quizId"`
	LessonId *string   `json:"lessonId"`
	Score    *int      `json:"score"`
	At       time.Time `json:"at"`
}
type commentRow struct {
	CommentId  string    `json:"commentId"`
	TargetType string    `json:"targetType"`
	TargetId   string    `json:"targetId"`
	At         time.Time `json:"at"`
}
type postRow struct {
	PostId string    `json:"postId"`
	At     time.Time `json:"at"`
}
type ratingRow struct {
	LessonId string    `json:"lessonId"`
	Rating   int       `json:"rating"`
	At       time.Time `json:"at"`
}

func buildStudentDetailsJSON(ctx context.Context, db *sql.DB, tenantId, userId string, from, to time.Time) ([]byte, error) {
	out := map[string]any{}

	// Top 100 lessons by watch seconds.
	var lessons []lessonAgg
	rows, err := db.QueryContext(ctx, `
		SELECT "lessonId", SUM("seconds")::int AS sec, MAX("createdAt") AS last_at
		FROM "LessonWatchEvent"
		WHERE "tenantId"=$1 AND "userId"=$2 AND "createdAt" >= $3 AND "createdAt" < $4
		GROUP BY "lessonId" ORDER BY sec DESC LIMIT 100
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l lessonAgg
		if err := rows.Scan(&l.LessonId, &l.Seconds, &l.LastAt); err != nil {
			rows.Close()
			return nil, err
		}
		lessons = append(lessons, l)
	}
	rows.Close()
	out["lessons"] = lessons

	// Exams (all).
	var exams []examRow
	rows, err = db.QueryContext(ctx, `
		SELECT "examId", "score", "passed", "createdAt" FROM "ExamCompletionEvent"
		WHERE "tenantId"=$1 AND "userId"=$2 AND "createdAt" >= $3 AND "createdAt" < $4
		ORDER BY "createdAt"
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e examRow
		if err := rows.Scan(&e.ExamId, &e.Score, &e.Passed, &e.At); err != nil {
			rows.Close()
			return nil, err
		}
		exams = append(exams, e)
	}
	rows.Close()
	out["exams"] = exams

	// Quizzes (cap 500).
	var quizzes []quizRow
	rows, err = db.QueryContext(ctx, `
		SELECT "quizId", NULL::text AS "lessonId", "score", "createdAt" FROM "StudentQuiz"
		WHERE "tenantId"=$1 AND "studentId"=$2 AND "createdAt" >= $3 AND "createdAt" < $4
		  AND "quizId" IS NOT NULL AND "score" IS NOT NULL AND "deletedAt" IS NULL
		ORDER BY "createdAt" DESC LIMIT 500
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var q quizRow
		if err := rows.Scan(&q.QuizId, &q.LessonId, &q.Score, &q.At); err != nil {
			rows.Close()
			return nil, err
		}
		quizzes = append(quizzes, q)
	}
	rows.Close()
	out["quizzes"] = quizzes

	// Comments (cap 500).
	var comments []commentRow
	rows, err = db.QueryContext(ctx, `
		SELECT "commentId", "targetType", "targetId", "createdAt" FROM "CommentEvent"
		WHERE "tenantId"=$1 AND "userId"=$2 AND "createdAt" >= $3 AND "createdAt" < $4
		ORDER BY "createdAt" DESC LIMIT 500
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c commentRow
		if err := rows.Scan(&c.CommentId, &c.TargetType, &c.TargetId, &c.At); err != nil {
			rows.Close()
			return nil, err
		}
		comments = append(comments, c)
	}
	rows.Close()
	out["comments"] = comments

	// Posts (cap 500).
	var posts []postRow
	rows, err = db.QueryContext(ctx, `
		SELECT "postId", "createdAt" FROM "CommunityPostEvent"
		WHERE "tenantId"=$1 AND "userId"=$2 AND "createdAt" >= $3 AND "createdAt" < $4
		ORDER BY "createdAt" DESC LIMIT 500
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p postRow
		if err := rows.Scan(&p.PostId, &p.At); err != nil {
			rows.Close()
			return nil, err
		}
		posts = append(posts, p)
	}
	rows.Close()
	out["posts"] = posts

	// Ratings (cap 500) — Read.createdAt as proxy for rating time.
	var ratings []ratingRow
	rows, err = db.QueryContext(ctx, `
		SELECT "lessonId", "rating", "createdAt" FROM "Read"
		WHERE "tenantId"=$1 AND "userId"=$2 AND "rating" IS NOT NULL
		  AND "createdAt" >= $3 AND "createdAt" < $4
		  AND "lessonId" IS NOT NULL
		ORDER BY "createdAt" DESC LIMIT 500
	`, tenantId, userId, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r ratingRow
		if err := rows.Scan(&r.LessonId, &r.Rating, &r.At); err != nil {
			rows.Close()
			return nil, err
		}
		ratings = append(ratings, r)
	}
	rows.Close()
	out["ratings"] = ratings

	return json.Marshal(out)
}

// deleteRawMonth wipes raw event rows for the closed month. Read is NOT deleted —
// it is the source of truth for lesson completion + rating state.
func (j *MonthlyRollupJob) deleteRawMonth(ctx context.Context, from, to time.Time) error {
	// safe: tables is a hardcoded allowlist, not user input — string concatenation here
	// is intentional and not a SQL injection vector.
	tables := []string{
		"LoginEvent", "LessonAccessEvent", "LessonWatchEvent",
		"ExamCompletionEvent", "CommentEvent", "CommunityPostEvent",
	}
	for _, t := range tables {
		q := fmt.Sprintf(`DELETE FROM %q WHERE "createdAt" >= $1 AND "createdAt" < $2`, t)
		if _, err := j.db.ExecContext(ctx, q, from, to); err != nil {
			return err
		}
	}
	return nil
}
