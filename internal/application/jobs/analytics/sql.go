package analytics

// SQL queries used by the analytics rollup jobs. CockroachDB-compatible.

// Per-tenant daily activity flags. One query per event source, all converging on
// (tenantId, day, userId). Bool columns use ON CONFLICT DO UPDATE SET column = true.
// "day" is computed in the tenant's timezone via DATE("createdAt" AT TIME ZONE $2).
// Read.lessonId is nullable; trackLessonRead always passes lessonId, so read=true rows
// with NULL lessonId cannot be produced by current writers — the filter readAt IS NOT NULL
// is sufficient.
var dailyActivitySQLs = []string{
	// login
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didLogin")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "LoginEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didLogin" = true`,

	// lesson access
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didLessonAccess")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "LessonAccessEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didLessonAccess" = true`,

	// lesson read — uses denormalized Read.tenantId + Read.readAt, no joins.
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didLessonRead","lessonReadCount")
	 SELECT $1, DATE("readAt" AT TIME ZONE $2), "userId", true, COUNT(*)::int
	 FROM "Read"
	 WHERE "tenantId" = $1
	   AND "read" = true
	   AND "readAt" IS NOT NULL
	   AND "readAt" >= $3 AND "readAt" < $4
	 GROUP BY DATE("readAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET
	   "didLessonRead" = true,
	   "lessonReadCount" = EXCLUDED."lessonReadCount"`,

	// watch
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didWatch","watchSec")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true, SUM("seconds")::int
	 FROM "LessonWatchEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET
	   "didWatch" = true,
	   "watchSec" = EXCLUDED."watchSec"`,

	// quiz
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didQuiz")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "QuizCompletionEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didQuiz" = true`,

	// exam
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didExam")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "ExamCompletionEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didExam" = true`,

	// comment
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didComment")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "CommentEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didComment" = true`,

	// community post
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didPost")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "CommunityPostEvent"
	 WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didPost" = true`,

	// rating — Read.createdAt is the proxy for rating time (no ratedAt column).
	// Known imprecision: if user read first and rated later, rating attributes to read day.
	`INSERT INTO "TenantDailyUserActivity" ("tenantId","day","userId","didRating")
	 SELECT $1, DATE("createdAt" AT TIME ZONE $2), "userId", true
	 FROM "Read"
	 WHERE "tenantId" = $1 AND "rating" IS NOT NULL AND "createdAt" >= $3 AND "createdAt" < $4
	 GROUP BY DATE("createdAt" AT TIME ZONE $2), "userId"
	 ON CONFLICT ("tenantId","day","userId") DO UPDATE SET "didRating" = true`,
}

const listAllTenantsSQL = `SELECT id, COALESCE("timezone", 'America/Sao_Paulo') FROM "Tenant"`

const tenantLocalDaysInRangeSQL = `
SELECT DISTINCT DATE("createdAt" AT TIME ZONE $2) AS d
FROM (
  SELECT "createdAt" FROM "LoginEvent"          WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "LessonAccessEvent"   WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "LessonWatchEvent"    WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "QuizCompletionEvent" WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "ExamCompletionEvent" WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "CommentEvent"        WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "createdAt" FROM "CommunityPostEvent"  WHERE "tenantId" = $1 AND "createdAt" >= $3 AND "createdAt" < $4
  UNION ALL SELECT "readAt"    FROM "Read"                WHERE "tenantId" = $1 AND "read" = true AND "readAt" IS NOT NULL AND "readAt" >= $3 AND "readAt" < $4
) u
ORDER BY d
`

// dailyStatsSQL upserts the per-day, per-tenant aggregate counters.
// $1 tenantId, $2 day (Date), $3 dayStart UTC, $4 dayEnd UTC, $5 details JSON.
const dailyStatsSQL = `
INSERT INTO "TenantDailyStats" (
  "tenantId","day","loginCount","lessonAccessCount","lessonReadCount","watchSeconds",
  "quizCount","examCount","examPassedCount","ratingCount","ratingAvg",
  "commentCount","communityPostCount","activeUserCount","details"
)
SELECT
  $1, $2,
  (SELECT COUNT(*) FROM "LoginEvent"          WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "LessonAccessEvent"   WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  COALESCE((SELECT SUM("lessonReadCount")::int FROM "TenantDailyUserActivity" WHERE "tenantId"=$1 AND "day"=$2), 0),
  COALESCE((SELECT SUM("watchSec")::int        FROM "TenantDailyUserActivity" WHERE "tenantId"=$1 AND "day"=$2), 0),
  (SELECT COUNT(*) FROM "QuizCompletionEvent"  WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "ExamCompletionEvent"  WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "ExamCompletionEvent"  WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4 AND "passed"=true),
  (SELECT COUNT(*)             FROM "Read" WHERE "tenantId"=$1 AND "rating" IS NOT NULL AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT AVG("rating")::float FROM "Read" WHERE "tenantId"=$1 AND "rating" IS NOT NULL AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "CommentEvent"        WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "CommunityPostEvent"  WHERE "tenantId"=$1 AND "createdAt" >= $3 AND "createdAt" < $4),
  (SELECT COUNT(*) FROM "TenantDailyUserActivity" WHERE "tenantId"=$1 AND "day"=$2),
  $5
ON CONFLICT ("tenantId","day") DO UPDATE SET
  "loginCount"=EXCLUDED."loginCount", "lessonAccessCount"=EXCLUDED."lessonAccessCount",
  "lessonReadCount"=EXCLUDED."lessonReadCount", "watchSeconds"=EXCLUDED."watchSeconds",
  "quizCount"=EXCLUDED."quizCount", "examCount"=EXCLUDED."examCount",
  "examPassedCount"=EXCLUDED."examPassedCount", "ratingCount"=EXCLUDED."ratingCount",
  "ratingAvg"=EXCLUDED."ratingAvg", "commentCount"=EXCLUDED."commentCount",
  "communityPostCount"=EXCLUDED."communityPostCount",
  "activeUserCount"=EXCLUDED."activeUserCount", "details"=EXCLUDED."details"
`
