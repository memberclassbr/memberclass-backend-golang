package comment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
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
            c."createdAt",
            c."updatedAt",
            c.published,
            c.question,
            c.answer,
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
		&response.CreatedAt,
		&response.UpdatedAt,
		&published,
		&response.Question,
		&answer,
		&response.LessonName,
		&response.CourseName,
		&response.Username,
		&response.UserEmail,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, memberclasserrors.ErrCommentNotFound
	}
	if err != nil {
		return nil, err
	}

	if answer.Valid && answer.String != "" {
		response.Answer = &answer.String
	}

	if published.Valid {
		response.Published = &published.Bool
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

func (r *CommentRepository) FindAllByTenant(ctx context.Context, tenantID string, req *request.GetCommentsRequest) ([]*dto.CommentResponse, int64, error) {
	baseQuery := `
        SELECT 
            c.id,
            c."createdAt",
            c."updatedAt",
            c.published,
            c.question,
            c.answer,
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

	args := []interface{}{tenantID}
	argIndex := 2

	// Filtro por email (ILIKE para case insensitive contains)
	if req.Email != nil && *req.Email != "" {
		baseQuery += fmt.Sprintf(` AND u.email ILIKE $%d`, argIndex)
		args = append(args, "%"+*req.Email+"%")
		argIndex++
	}

	// Filtro por status
	if req.Status != nil && *req.Status != "" {
		status := strings.ToLower(*req.Status)
		if status == "pendent" {
			baseQuery += ` AND c.published IS NULL`
		} else if status == "approved" {
			baseQuery += fmt.Sprintf(` AND c.published = $%d`, argIndex)
			args = append(args, true)
			argIndex++
		} else if status == "rejected" {
			baseQuery += fmt.Sprintf(` AND c.published = $%d`, argIndex)
			args = append(args, false)
			argIndex++
		}
	}

	// Filtro por courseId
	if req.CourseID != nil && *req.CourseID != "" {
		baseQuery += fmt.Sprintf(` AND course.id = $%d`, argIndex)
		args = append(args, *req.CourseID)
		argIndex++
	}

	// Filtro por answered
	if req.Answered != nil && *req.Answered != "" {
		answered := strings.ToLower(*req.Answered)
		if answered == "true" {
			baseQuery += ` AND c.answer IS NOT NULL AND c.answer != ''`
		} else if answered == "false" {
			baseQuery += ` AND (c.answer IS NULL OR c.answer = '')`
		}
	}

	// Ordenação e paginação
	baseQuery += ` ORDER BY c."createdAt" DESC`
	skip := (req.Page - 1) * req.Limit
	baseQuery += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIndex, argIndex+1)
	args = append(args, req.Limit, skip)

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		r.log.Error("Error finding comments: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding comments",
		}
	}
	defer rows.Close()

	var comments []*dto.CommentResponse
	for rows.Next() {
		var response dto.CommentResponse
		var answer sql.NullString
		var published sql.NullBool

		err := rows.Scan(
			&response.ID,
			&response.CreatedAt,
			&response.UpdatedAt,
			&published,
			&response.Question,
			&answer,
			&response.LessonName,
			&response.CourseName,
			&response.Username,
			&response.UserEmail,
		)
		if err != nil {
			r.log.Error("Error scanning comment: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning comment",
			}
		}

		if answer.Valid && answer.String != "" {
			response.Answer = &answer.String
		}

		if published.Valid {
			response.Published = &published.Bool
		}

		comments = append(comments, &response)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating comments: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating comments",
		}
	}

	// Query de contagem
	countQuery := `
        SELECT COUNT(*)
        FROM "Comment" c
        JOIN "Lesson" l ON c."lessonId" = l.id
        JOIN "Module" m ON l."moduleId" = m.id
        JOIN "Section" s ON m."sectionId" = s.id
        JOIN "Course" course ON s."courseId" = course.id
        JOIN "Vitrine" v ON course."vitrineId" = v.id
        JOIN "User" u ON c."userId" = u.id
        WHERE v."tenantId" = $1
    `

	countArgs := []interface{}{tenantID}
	countArgIndex := 2

	if req.Email != nil && *req.Email != "" {
		countQuery += fmt.Sprintf(` AND u.email ILIKE $%d`, countArgIndex)
		countArgs = append(countArgs, "%"+*req.Email+"%")
		countArgIndex++
	}

	if req.Status != nil && *req.Status != "" {
		status := strings.ToLower(*req.Status)
		if status == "pendent" {
			countQuery += ` AND c.published IS NULL`
		} else if status == "approved" {
			countQuery += fmt.Sprintf(` AND c.published = $%d`, countArgIndex)
			countArgs = append(countArgs, true)
			countArgIndex++
		} else if status == "rejected" {
			countQuery += fmt.Sprintf(` AND c.published = $%d`, countArgIndex)
			countArgs = append(countArgs, false)
			countArgIndex++
		}
	}

	if req.CourseID != nil && *req.CourseID != "" {
		countQuery += fmt.Sprintf(` AND course.id = $%d`, countArgIndex)
		countArgs = append(countArgs, *req.CourseID)
		countArgIndex++
	}

	if req.Answered != nil && *req.Answered != "" {
		answered := strings.ToLower(*req.Answered)
		if answered == "true" {
			countQuery += ` AND c.answer IS NOT NULL AND c.answer != ''`
		} else if answered == "false" {
			countQuery += ` AND (c.answer IS NULL OR c.answer = '')`
		}
	}

	var total int64
	err = r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		r.log.Error("Error counting comments: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting comments",
		}
	}

	return comments, total, nil
}
