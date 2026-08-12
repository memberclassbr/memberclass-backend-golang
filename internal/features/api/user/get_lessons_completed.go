package user

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/shared/pagination"
	"github.com/memberclass-backend-golang/internal/shared/tenant"

	"github.com/memberclass-backend-golang/internal/shared/datefilter"
)

// ---------- DTOs ----------

type completedLesson struct {
	CourseName  string `json:"courseName"`
	LessonName  string `json:"lessonName"`
	CompletedAt string `json:"completedAt"`
}

type lessonsCompletedData struct {
	CompletedLessons []completedLesson `json:"completedLessons"`
	Pagination       pagination.Meta   `json:"pagination"`
}

// lessonsCompletedResponse wraps its payload in {ok, data}, unlike the other
// two actions in this slice. That is the shape the endpoint has always had.
type lessonsCompletedResponse struct {
	OK   bool                 `json:"ok"`
	Data lessonsCompletedData `json:"data"`
}

type lessonsCompletedRequest struct {
	Email     string
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
	CourseID  string
}

const (
	errEmailRequired    = "email é obrigatório"
	errPageNumber       = "page deve ser um número"
	errLimitNumber      = "limit deve ser um número"
	errPageRange        = "page deve ser >= 1"
	errLimitRange       = "limit deve ser entre 1 e 100"
	errStartDateFormat  = "formato de data inválido para startDate. Use YYYY-MM-DD ou ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)"
	errEndDateFormat    = "formato de data inválido para endDate. Use YYYY-MM-DD ou ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)"
	errStartRequired    = "data de início é obrigatória quando data final é fornecida"
	errStartAfterEnd    = "a data de início não pode ser maior que a data de fim"
	errWindowTooWide    = "período máximo de 31 dias"
	maxCompletedWindow  = 31 * 24 * time.Hour
	defaultWindowInDays = 31
)

// ---------- 1. HTTP handler ----------

// GetLessonsCompleted handles `GET /api/v1/user/lessons/completed`.
func (f *Feature) GetLessonsCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "API key invalid", "INVALID_API_KEY")
		return
	}

	req, err := parseLessonsCompleted(r.URL.Query())
	if err != nil {
		writeCustomError(w, http.StatusBadRequest, err.Error(), parseErrorCode(err))
		return
	}
	if err := req.validate(); err != nil {
		message, code := validationErrorResponse(err)
		writeCustomError(w, http.StatusBadRequest, message, code)
		return
	}

	resp, err := f.getLessonsCompleted(r.Context(), tenant.ID, *req)
	if err != nil {
		if errors.Is(err, errUserNotInTenant) {
			writeCustomError(w, http.StatusNotFound, "Usuário não encontrado ou não pertence a este tenant", "USER_NOT_IN_TENANT")
			return
		}
		f.writeUnexpected(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseLessonsCompleted(query url.Values) (*lessonsCompletedRequest, error) {
	req := &lessonsCompletedRequest{
		Email:    query.Get("email"),
		Page:     1,
		Limit:    10,
		CourseID: query.Get("courseId"),
	}

	if v := query.Get("page"); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New(errPageNumber)
		}
		req.Page = page
	}

	if v := query.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New(errLimitNumber)
		}
		req.Limit = limit
	}

	if v := query.Get("startDate"); v != "" {
		startDate, err := datefilter.Parse(v, datefilter.StartOfDay)
		if err != nil {
			return nil, errors.New(errStartDateFormat)
		}
		req.StartDate = &startDate
	}

	if v := query.Get("endDate"); v != "" {
		endDate, err := datefilter.Parse(v, datefilter.EndOfDay)
		if err != nil {
			return nil, errors.New(errEndDateFormat)
		}
		req.EndDate = &endDate
	}

	return req, nil
}

func (r *lessonsCompletedRequest) validate() error {
	if r.Email == "" {
		return errors.New(errEmailRequired)
	}
	if r.Page < 1 {
		return errors.New(errPageRange)
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New(errLimitRange)
	}
	if r.EndDate != nil && r.StartDate == nil {
		return errors.New(errStartRequired)
	}
	if r.StartDate != nil && r.EndDate != nil {
		if r.StartDate.After(*r.EndDate) {
			return errors.New(errStartAfterEnd)
		}
		if r.EndDate.Sub(*r.StartDate) > maxCompletedWindow {
			return errors.New(errWindowTooWide)
		}
	}
	return nil
}

func parseErrorCode(err error) string {
	switch err.Error() {
	case errPageNumber, errLimitNumber:
		return "INVALID_PAGINATION"
	case errStartDateFormat, errEndDateFormat:
		return "INVALID_DATE_FORMAT"
	default:
		return "INVALID_REQUEST"
	}
}

func validationErrorResponse(err error) (message, code string) {
	switch err.Error() {
	case errEmailRequired:
		return "Parâmetro email é obrigatório", "MISSING_EMAIL"
	case errPageRange, errLimitRange:
		return err.Error(), "INVALID_PAGINATION"
	case errStartRequired, errStartAfterEnd, errWindowTooWide:
		return err.Error(), "INVALID_DATE_RANGE"
	default:
		return err.Error(), "INVALID_REQUEST"
	}
}

// ---------- 2. Business rule ----------

func (f *Feature) getLessonsCompleted(ctx context.Context, tenantID string, req lessonsCompletedRequest) (*lessonsCompletedResponse, error) {
	userID, err := f.memberID(ctx, req.Email, tenantID)
	if err != nil {
		return nil, err
	}

	startDate, endDate := resolveWindow(req, time.Now())

	lessons, total, err := f.queryCompletedLessons(ctx, userID, tenantID, startDate, endDate, req.CourseID, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	return &lessonsCompletedResponse{
		OK: true,
		Data: lessonsCompletedData{
			CompletedLessons: lessons,
			Pagination:       pagination.NewMeta(req.Page, req.Limit, total),
		},
	}, nil
}

// resolveWindow applies the slice's date policy. The policy itself lives in
// datefilter, shared with /user/activities and /user/activity/summary: all
// three advertise "last 31 days" and used to measure it two different ways.
func resolveWindow(req lessonsCompletedRequest, now time.Time) (start, end time.Time) {
	return datefilter.ResolveWindow(req.StartDate, req.EndDate, now, defaultWindowInDays)
}

// ---------- 3. SQL ----------

const (
	// sqlCompletedLessons resolves the member's reads to lessons, then keeps
	// only those belonging to the tenant. %s is the optional course filter.
	//
	// The tenant scope walks lesson → module → section → course → vitrine,
	// because Vitrine is what a Tenant owns. It used to walk through
	// CourseOnDelivery → Delivery instead, which asks a different question: not
	// "does this lesson belong to the tenant" but "is this lesson's course
	// bundled into one of its offers". A course with no CourseOnDelivery row —
	// or one granted at vitrine, module or lesson level, since deliveries can
	// bind at any of those — matched nothing, and the endpoint returned an
	// empty page for a member who had genuinely completed lessons.
	//
	// /api/v1/user/activities has always used the vitrine chain for the same
	// reads, which is why it listed a completion this endpoint could not see.
	sqlCompletedLessons = `
		WITH completed_reads AS (
			SELECT r."createdAt", r."lessonId"
			FROM "Read" r
			WHERE r."userId" = $1
			  AND r.read = true
			  AND r."lessonId" IS NOT NULL
			  AND r."createdAt" >= $2
			  AND r."createdAt" <= $3
		),
		lessons_in_tenant AS (
			SELECT DISTINCT
				l.id as lesson_id,
				l.name as lesson_name,
				c.id as course_id,
				c.name as course_name
			FROM "Lesson" l
			JOIN "Module" m ON m.id = l."moduleId"
			JOIN "Section" s ON s.id = m."sectionId"
			JOIN "Course" c ON c.id = s."courseId"
			JOIN "Vitrine" v ON v.id = c."vitrineId"
			WHERE l.id IN (SELECT "lessonId" FROM completed_reads)
			  AND v."tenantId" = $4%s
		)
		SELECT
			cr."createdAt" as completed_at,
			lit.lesson_name,
			lit.course_name
		FROM completed_reads cr
		JOIN lessons_in_tenant lit ON lit.lesson_id = cr."lessonId"
		ORDER BY cr."createdAt" DESC
		LIMIT $%d OFFSET $%d
	`

	sqlCountCompletedLessons = `
		SELECT COUNT(DISTINCT r."lessonId")
		FROM "Read" r
		JOIN "Lesson" l ON l.id = r."lessonId"
		JOIN "Module" m ON m.id = l."moduleId"
		JOIN "Section" s ON s.id = m."sectionId"
		JOIN "Course" c ON c.id = s."courseId"
		JOIN "Vitrine" v ON v.id = c."vitrineId"
		WHERE r."userId" = $1
		  AND r.read = true
		  AND r."lessonId" IS NOT NULL
		  AND r."createdAt" >= $2
		  AND r."createdAt" <= $3
		  AND v."tenantId" = $4
	`
)

func (f *Feature) queryCompletedLessons(
	ctx context.Context,
	userID, tenantID string,
	startDate, endDate time.Time,
	courseID string,
	page, limit int,
) ([]completedLesson, int64, error) {
	args := []any{userID, startDate, endDate, tenantID}

	courseClause := ""
	if courseID != "" {
		courseClause = ` AND c.id = $5`
		args = append(args, courseID)
	}

	query := fmt.Sprintf(sqlCompletedLessons, courseClause, len(args)+1, len(args)+2)
	args = append(args, limit, pagination.Offset(page, limit))

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, f.fail("Error finding completed lessons: ", err, "error finding completed lessons")
	}
	defer rows.Close()

	lessons := make([]completedLesson, 0)
	for rows.Next() {
		var lesson completedLesson
		var completedAt time.Time

		if err := rows.Scan(&completedAt, &lesson.LessonName, &lesson.CourseName); err != nil {
			return nil, 0, f.fail("Error scanning completed lesson: ", err, "error scanning completed lesson")
		}
		lesson.CompletedAt = completedAt.Format(timestampLayout)
		lessons = append(lessons, lesson)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, f.fail("Error iterating completed lessons: ", err, "error iterating completed lessons")
	}

	total, err := f.countCompletedLessons(ctx, userID, tenantID, startDate, endDate, courseID)
	if err != nil {
		return nil, 0, err
	}

	return lessons, total, nil
}

func (f *Feature) countCompletedLessons(
	ctx context.Context,
	userID, tenantID string,
	startDate, endDate time.Time,
	courseID string,
) (int64, error) {
	query := sqlCountCompletedLessons
	args := []any{userID, startDate, endDate, tenantID}
	if courseID != "" {
		query += ` AND c.id = $5`
		args = append(args, courseID)
	}

	var total int64
	if err := f.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, f.fail("Error counting completed lessons: ", err, "error counting completed lessons")
	}
	return total, nil
}
