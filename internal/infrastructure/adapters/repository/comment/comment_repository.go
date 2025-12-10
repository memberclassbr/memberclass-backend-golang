package comment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
)

type CommentRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewCommentRepository(db *sql.DB, log ports.Logger) ports.CommentRepository {
	return &CommentRepository{
		db:  db,
		log: log,
	}
}

func (r *CommentRepository) FindByIDAndTenant(ctx context.Context, commentID, tenantID string) (*entities.Comment, error) {
	query := `
        SELECT 
            c.id,
            c.question,
            c.answer,
            c.published,
            c."createdAt",
            c."updatedAt",
            c."lessonId",
            c."userId"
        FROM "Comment" c
        JOIN "Lesson" l ON c."lessonId" = l.id
        JOIN "Module" m ON l."moduleId" = m.id
        JOIN "Section" s ON m."sectionId" = s.id
        JOIN "Course" course ON s."courseId" = course.id
        JOIN "Vitrine" v ON course."vitrineId" = v.id
        WHERE c.id = $1 AND v."tenantId" = $2
        LIMIT 1
    `

	var comment entities.Comment
	var answer sql.NullString
	var published sql.NullBool

	err := r.db.QueryRowContext(ctx, query, commentID, tenantID).Scan(
		&comment.ID,
		&comment.Question,
		&answer,
		&published,
		&comment.CreatedAt,
		&comment.UpdatedAt,
		&comment.LessonID,
		&comment.UserID,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, memberclasserrors.ErrCommentNotFound
	}
	if err != nil {
		return nil, err
	}

	if answer.Valid {
		comment.Answer = &answer.String
	}
	if published.Valid {
		comment.Published = published.Bool
	}

	return &comment, nil
}

func (r *CommentRepository) FindByIDAndTenantWithDetails(ctx context.Context, commentID, tenantID string) (*dto.CommentResponse, error) {
	query := `
        SELECT 
            c.id,
            c.question,
            c.answer,
            c.published,
            c."updatedAt",
            l.name as lesson_name,
            course.name as course_name,
            COALESCE(uot.name, '') as user_name,
            u.email as user_email
        FROM "Comment" c
        JOIN "Lesson" l ON c."lessonId" = l.id
        JOIN "Module" m ON l."moduleId" = m.id
        JOIN "Section" s ON m."sectionId" = s.id
        JOIN "Course" course ON s."courseId" = course.id
        JOIN "Vitrine" v ON course."vitrineId" = v.id
        JOIN "User" u ON c."userId" = u.id
        LEFT JOIN "UsersOnTenants" uot ON u.id = uot."userId" AND uot."tenantId" = $2
        WHERE c.id = $1 AND v."tenantId" = $2
        LIMIT 1
    `

	var response dto.CommentResponse
	var answer sql.NullString
	var published sql.NullBool

	err := r.db.QueryRowContext(ctx, query, commentID, tenantID).Scan(
		&response.ID,
		&response.Question,
		&answer,
		&published,
		&response.UpdatedAt,
		&response.LessonName,
		&response.CourseName,
		&response.UserName,
		&response.UserEmail,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, memberclasserrors.ErrCommentNotFound
	}
	if err != nil {
		return nil, err
	}

	if answer.Valid {
		response.Answer = answer.String
	}
	if published.Valid {
		response.Published = published.Bool
	}

	return &response, nil
}

func (r *CommentRepository) Update(ctx context.Context, commentID, answer string, published bool) (*entities.Comment, error) {
	query := `
        UPDATE "Comment"
        SET answer = $2, published = $3, "updatedAt" = $4
        WHERE id = $1
        RETURNING id, question, answer, published, "updatedAt"
    `

	now := time.Now()
	var comment entities.Comment
	var answerResult sql.NullString
	var publishedResult sql.NullBool

	err := r.db.QueryRowContext(ctx, query, commentID, answer, published, now).Scan(
		&comment.ID,
		&comment.Question,
		&answerResult,
		&publishedResult,
		&comment.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if answerResult.Valid {
		comment.Answer = &answerResult.String
	}
	if publishedResult.Valid {
		comment.Published = publishedResult.Bool
	}

	return &comment, nil
}

func (r *CommentRepository) FindAllByTenant(ctx context.Context, tenantID string, pagination *dto.PaginationRequest) ([]*dto.CommentResponse, int64, error) {
	paginationUtils := usecases.NewPaginationUtils()
	paginationUtils.ValidatePaginationRequest(pagination)

	sortBy := pagination.GetSortBy()
	sortByWithPrefix := fmt.Sprintf("c.\"%s\"", sortBy)

	baseQuery := `
        SELECT 
            c.id,
            c.question,
            c.answer,
            c.published,
            c."updatedAt",
            l.name as lesson_name,
            course.name as course_name,
            COALESCE(uot.name, '') as user_name,
            u.email as user_email
        FROM "Comment" c
        JOIN "Lesson" l ON c."lessonId" = l.id
        JOIN "Module" m ON l."moduleId" = m.id
        JOIN "Section" s ON m."sectionId" = s.id
        JOIN "Course" course ON s."courseId" = course.id
        JOIN "Vitrine" v ON course."vitrineId" = v.id
        JOIN "User" u ON c."userId" = u.id
        LEFT JOIN "UsersOnTenants" uot ON u.id = uot."userId" AND uot."tenantId" = $1
        WHERE v."tenantId" = $1
    `

	tempPagination := &dto.PaginationRequest{
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
		SortBy:   sortByWithPrefix,
		SortDir:  pagination.SortDir,
	}

	queryWithPagination := paginationUtils.BuildSQLPagination(baseQuery, tempPagination)
	
	limit := pagination.GetLimit()
	offset := pagination.GetOffset()
	query := strings.ReplaceAll(queryWithPagination, fmt.Sprintf("LIMIT %d", limit), "LIMIT $2")
	query = strings.ReplaceAll(query, fmt.Sprintf("OFFSET %d", offset), "OFFSET $3")

	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var comments []*dto.CommentResponse
	for rows.Next() {
		var response dto.CommentResponse
		var answer sql.NullString
		var published sql.NullBool

		err := rows.Scan(
			&response.ID,
			&response.Question,
			&answer,
			&published,
			&response.UpdatedAt,
			&response.LessonName,
			&response.CourseName,
			&response.UserName,
			&response.UserEmail,
		)
		if err != nil {
			return nil, 0, err
		}

		if answer.Valid {
			response.Answer = answer.String
		}
		if published.Valid {
			response.Published = published.Bool
		}

		comments = append(comments, &response)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	countQuery := paginationUtils.BuildCountQuery(baseQuery)

	var total int64
	err = r.db.QueryRowContext(ctx, countQuery, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}
