package comment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/pagination"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// ---------- DTOs ----------

type commentResponse struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LessonName string    `json:"lessonName"`
	CourseName string    `json:"courseName"`
	Published  *bool     `json:"published"`
	Question   string    `json:"question"`
	Answer     *string   `json:"answer"`
	Username   string    `json:"username"`
	UserEmail  string    `json:"userEmail"`
}

type commentsPaginationResponse struct {
	Comments   []commentResponse `json:"comments"`
	Pagination pagination.Meta   `json:"pagination"`
}

type getCommentsRequest struct {
	Page     int
	Limit    int
	Email    string
	Status   string
	CourseID string
	Answered string
}

// ---------- 1. HTTP handler ----------

// GetComments handles `GET /api/v1/comments`.
func (f *Feature) GetComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Both routes are authenticated, so a missing tenant means the middleware
	// did not populate the context. The previous handler only checked this on
	// the /api/v1 path and dereferenced the nil tenant on the other one.
	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	req, err := parseGetComments(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidPagination, "INVALID_PAGINATION")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidPagination, "INVALID_PAGINATION")
		return
	}

	resp, err := f.getComments(r.Context(), tenant.ID, *req)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

const msgInvalidPagination = "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100"

func parseGetComments(query url.Values) (*getCommentsRequest, error) {
	req := &getCommentsRequest{
		Page:     1,
		Limit:    10,
		Email:    query.Get("email"),
		Status:   query.Get("status"),
		CourseID: query.Get("courseId"),
		Answered: query.Get("answered"),
	}

	if v := query.Get("page"); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("page deve ser um número")
		}
		req.Page = page
	}

	if v := query.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("limit deve ser um número")
		}
		req.Limit = limit
	}

	return req, nil
}

// validate rejects out-of-range pagination and *silently drops* unrecognised
// status and answered values rather than failing the request. That leniency is
// deliberate: these filters come from UI dropdowns, and an unknown value should
// widen the result set, not break the page.
func (r *getCommentsRequest) validate() error {
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}

	switch strings.ToLower(r.Status) {
	case "pendent", "approved", "rejected":
		r.Status = strings.ToLower(r.Status)
	default:
		r.Status = ""
	}

	switch strings.ToLower(r.Answered) {
	case "true", "false":
		r.Answered = strings.ToLower(r.Answered)
	default:
		r.Answered = ""
	}

	return nil
}

// ---------- 2. Business rule ----------

func (f *Feature) getComments(ctx context.Context, tenantID string, req getCommentsRequest) (*commentsPaginationResponse, error) {
	comments, total, err := f.queryComments(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	return &commentsPaginationResponse{
		Comments:   comments,
		Pagination: pagination.NewMeta(req.Page, req.Limit, total),
	}, nil
}

// ---------- 3. SQL ----------

const (
	// sqlCommentsSelect and sqlCommentsCount share the join chain that walks a
	// comment up to its Vitrine, which is the only level carrying a tenantId.
	sqlCommentsFrom = `
        FROM "Comment" c
        JOIN "Lesson" l ON c."lessonId" = l.id
        JOIN "Module" m ON l."moduleId" = m.id
        JOIN "Section" s ON m."sectionId" = s.id
        JOIN "Course" course ON s."courseId" = course.id
        JOIN "Vitrine" v ON course."vitrineId" = v.id
        JOIN "User" u ON c."userId" = u.id`

	sqlCommentsSelect = `
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
            u.email as user_email` + sqlCommentsFrom + `
        LEFT JOIN "UsersOnTenants" uot ON u.id = uot."userId" AND uot."tenantId" = $1
        WHERE v."tenantId" = $1`

	sqlCommentsCount = `SELECT COUNT(*)` + sqlCommentsFrom + `
        WHERE v."tenantId" = $1`
)

// commentFilters renders the optional WHERE clauses shared by the page and
// count queries. Keeping them in one place is what stops the two from drifting
// apart and reporting a total that does not match the rows.
func commentFilters(req getCommentsRequest, args []any) (string, []any) {
	clause := ""

	if req.Email != "" {
		// Substring match: the listing UI searches by partial address.
		clause += " AND u.email ILIKE $" + strconv.Itoa(len(args)+1)
		args = append(args, "%"+req.Email+"%")
	}

	switch req.Status {
	case "pendent":
		// Not yet moderated: published is still NULL.
		clause += ` AND c.published IS NULL`
	case "approved":
		clause += " AND c.published = $" + strconv.Itoa(len(args)+1)
		args = append(args, true)
	case "rejected":
		clause += " AND c.published = $" + strconv.Itoa(len(args)+1)
		args = append(args, false)
	}

	if req.CourseID != "" {
		clause += " AND course.id = $" + strconv.Itoa(len(args)+1)
		args = append(args, req.CourseID)
	}

	switch req.Answered {
	case "true":
		clause += ` AND c.answer IS NOT NULL AND c.answer != ''`
	case "false":
		clause += ` AND (c.answer IS NULL OR c.answer = '')`
	}

	return clause, args
}

func (f *Feature) queryComments(ctx context.Context, tenantID string, req getCommentsRequest) ([]commentResponse, int64, error) {
	args := []any{tenantID}
	filters, args := commentFilters(req, args)

	query := sqlCommentsSelect + filters +
		` ORDER BY c."createdAt" DESC LIMIT $` + strconv.Itoa(len(args)+1) +
		` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, req.Limit, pagination.Offset(req.Page, req.Limit))

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, f.fail("Error finding comments: ", err, "error finding comments")
	}
	defer rows.Close()

	comments := make([]commentResponse, 0)
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, 0, f.fail("Error scanning comment: ", err, "error scanning comment")
		}
		comments = append(comments, c)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, f.fail("Error iterating comments: ", err, "error iterating comments")
	}

	total, err := f.countComments(ctx, tenantID, req)
	if err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

func (f *Feature) countComments(ctx context.Context, tenantID string, req getCommentsRequest) (int64, error) {
	args := []any{tenantID}
	filters, args := commentFilters(req, args)

	var total int64
	if err := f.db.QueryRowContext(ctx, sqlCommentsCount+filters, args...).Scan(&total); err != nil {
		return 0, f.fail("Error counting comments: ", err, "error counting comments")
	}
	return total, nil
}

// scanRow is satisfied by both *sql.Row and *sql.Rows.
type scanRow interface {
	Scan(dest ...any) error
}

func scanComment(row scanRow) (commentResponse, error) {
	var c commentResponse
	var answer sql.NullString
	var published sql.NullBool

	err := row.Scan(
		&c.ID,
		&c.CreatedAt,
		&c.UpdatedAt,
		&published,
		&c.Question,
		&answer,
		&c.LessonName,
		&c.CourseName,
		&c.Username,
		&c.UserEmail,
	)
	if err != nil {
		return commentResponse{}, err
	}

	// An empty answer reads as "unanswered", not as an empty string.
	if answer.Valid && answer.String != "" {
		c.Answer = &answer.String
	}
	if published.Valid {
		c.Published = &published.Bool
	}

	return c, nil
}

// ---------- errors and responses ----------

func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}
