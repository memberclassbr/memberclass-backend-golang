package analytics

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/memberclass-backend-golang/internal/platform/logger"
)

// BackfillExtras populates CommentEvent and CommunityPostEvent from their primary
// entities (Comment, Post). Quiz no longer has its own event table — analytics
// queries read StudentQuiz directly. Idempotent via ON CONFLICT (id) DO NOTHING.
// Run after the main Backfill so the daily/monthly rollups can pick the data up
// next time.
//
// When tenantId is non-empty the run is scoped to that single tenant.
// Cursor pagination by primary-entity id keeps each transaction below the
// CockroachDB lock-tracking budget.
func BackfillExtras(ctx context.Context, db *sql.DB, logger logger.Logger, tenantId string) error {
	logger.Info("analytics backfill-extras start", "tenantId", tenantId)

	if err := backfillCommentFromComment(ctx, db, logger, tenantId); err != nil {
		return fmt.Errorf("comment: %w", err)
	}
	if err := backfillPostFromPost(ctx, db, logger, tenantId); err != nil {
		return fmt.Errorf("post: %w", err)
	}

	logger.Info("analytics backfill-extras done")
	return nil
}

const extrasChunkSize = 5_000

func backfillCommentFromComment(ctx context.Context, db *sql.DB, logger logger.Logger, tenantId string) error {
	cursor := ""
	total := int64(0)
	for {
		args := []any{cursor, extrasChunkSize}
		chunkTenant := ""
		if tenantId != "" {
			chunkTenant = `AND EXISTS (
				SELECT 1 FROM "Lesson" l, "Module" m, "Section" s, "Course" cr, "Vitrine" v
				WHERE l."id" = c."lessonId" AND m."id" = l."moduleId" AND s."id" = m."sectionId"
				  AND cr."id" = s."courseId" AND v."id" = cr."vitrineId" AND v."tenantId" = $3
			)`
			args = append(args, tenantId)
		}
		query := fmt.Sprintf(`
			WITH chunk AS (
				SELECT c."id" FROM "Comment" c
				WHERE c."id" > $1
				  %s
				ORDER BY c."id" LIMIT $2
			),
			ins AS (
				INSERT INTO "CommentEvent" ("id","tenantId","userId","targetType","targetId","commentId","createdAt")
				SELECT c."id", v."tenantId", c."userId", 'lesson', c."lessonId", c."id", c."createdAt"
				FROM "Comment" c, "Lesson" l, "Module" m, "Section" s, "Course" cr, "Vitrine" v
				WHERE c."id" IN (SELECT "id" FROM chunk)
				  AND l."id" = c."lessonId"
				  AND m."id" = l."moduleId"
				  AND s."id" = m."sectionId"
				  AND cr."id" = s."courseId"
				  AND v."id" = cr."vitrineId"
				ON CONFLICT ("id") DO NOTHING
				RETURNING 1
			)
			SELECT (SELECT MAX("id") FROM chunk), (SELECT COUNT(*) FROM ins)
		`, chunkTenant)
		var maxId sql.NullString
		var inserted int64
		if err := db.QueryRowContext(ctx, query, args...).Scan(&maxId, &inserted); err != nil {
			return err
		}
		total += inserted
		logger.Info("comment backfill batch", "inserted", inserted, "total", total, "lastId", maxId.String)
		if !maxId.Valid {
			break
		}
		cursor = maxId.String
	}
	logger.Info("comment backfill done", "total", total)
	return nil
}

func backfillPostFromPost(ctx context.Context, db *sql.DB, logger logger.Logger, tenantId string) error {
	cursor := ""
	total := int64(0)
	for {
		args := []any{cursor, extrasChunkSize}
		chunkTenant := ""
		if tenantId != "" {
			chunkTenant = `AND EXISTS (
				SELECT 1 FROM "Topic" t, "Category" c, "Social" s
				WHERE t."id" = p."topicId" AND c."id" = t."categoryId" AND s."id" = c."socialId" AND s."tenantId" = $3
			)`
			args = append(args, tenantId)
		}
		query := fmt.Sprintf(`
			WITH chunk AS (
				SELECT p."id" FROM "Post" p
				WHERE p."id" > $1
				  %s
				ORDER BY p."id" LIMIT $2
			),
			ins AS (
				INSERT INTO "CommunityPostEvent" ("id","tenantId","userId","postId","createdAt")
				SELECT p."id", s."tenantId", p."userId", p."id", p."createdAt"
				FROM "Post" p, "Topic" t, "Category" c, "Social" s
				WHERE p."id" IN (SELECT "id" FROM chunk)
				  AND t."id" = p."topicId"
				  AND c."id" = t."categoryId"
				  AND s."id" = c."socialId"
				ON CONFLICT ("id") DO NOTHING
				RETURNING 1
			)
			SELECT (SELECT MAX("id") FROM chunk), (SELECT COUNT(*) FROM ins)
		`, chunkTenant)
		var maxId sql.NullString
		var inserted int64
		if err := db.QueryRowContext(ctx, query, args...).Scan(&maxId, &inserted); err != nil {
			return err
		}
		total += inserted
		logger.Info("post backfill batch", "inserted", inserted, "total", total, "lastId", maxId.String)
		if !maxId.Valid {
			break
		}
		cursor = maxId.String
	}
	logger.Info("post backfill done", "total", total)
	return nil
}
