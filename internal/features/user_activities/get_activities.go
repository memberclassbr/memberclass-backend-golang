package user_activities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
)

// errUserNotFoundOrNotInTenant is the slice's sentinel: the email does not
// resolve to any user, or that user does not belong to the authenticated
// tenant. Both collapse to the same client-facing USER_NOT_FOUND response.
//
// Event "type" values returned in the JSON response are hardcoded inside the
// SQL UNION below: "login", "acceptTerms", "lessonCompleted", "download",
// "comment", "quiz", "certificate".
var errUserNotFoundOrNotInTenant = errors.New("user not found or not in tenant")

// ---------- DTOs ----------

type getActivitiesRequest struct {
	Email     string
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
}

type event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Date    time.Time       `json:"date"`
	Details json.RawMessage `json:"details"`
}

type activitiesResponse struct {
	Email      string             `json:"email"`
	Events     []event            `json:"events"`
	Pagination dto.PaginationMeta `json:"pagination"`
}

// ---------- 1. HTTP handler ----------

// GetActivities handles `GET /api/v1/user/activities`.
func (f *Feature) GetActivities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), "")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "API key invalid", "INVALID_API_KEY")
		return
	}

	req, err := parseRequest(r.URL.Query())
	if err != nil {
		writeCustomError(w, http.StatusBadRequest, err.Error(), classifyParseError(err))
		return
	}
	if err := req.validate(); err != nil {
		writeCustomError(w, http.StatusBadRequest, err.Error(), classifyValidationError(err))
		return
	}

	resp, err := f.getActivities(r.Context(), *req, tenant.ID)
	if err != nil {
		if errors.Is(err, errUserNotFoundOrNotInTenant) {
			writeCustomError(w, http.StatusNotFound, "Usuário não encontrado ou não pertence ao tenant autenticado", "USER_NOT_FOUND")
			return
		}
		var mcErr *memberclasserrors.MemberClassError
		if errors.As(err, &mcErr) {
			writeError(w, mcErr.Code, http.StatusText(mcErr.Code), mcErr.Message)
			return
		}
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), "")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) getActivities(ctx context.Context, req getActivitiesRequest, tenantID string) (*activitiesResponse, error) {
	userID, err := f.resolveUserID(ctx, req.Email, tenantID)
	if err != nil {
		return nil, err
	}

	startDate, endDate := resolveDateRange(req)
	cacheKey := buildCacheKey(tenantID, userID, req, startDate, endDate)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit activitiesResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	events, err := f.queryEvents(ctx, userID, tenantID, startDate, endDate, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalCount, err := f.countEvents(ctx, userID, tenantID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	resp := &activitiesResponse{
		Email:      req.Email,
		Events:     events,
		Pagination: buildPaginationMeta(req.Page, req.Limit, totalCount),
	}

	if raw, err := json.Marshal(resp); err == nil {
		if err := f.cache.Set(ctx, cacheKey, string(raw), 5*time.Minute); err != nil {
			f.log.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			f.log.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return resp, nil
}

// resolveUserID looks up the user by email, scoped to the tenant.
// This single query replaces the old "FindByEmail + BelongsToTenant" pair
// AND closes the cross-tenant leak (user existed in tenant A would return a
// userId the caller in tenant B then queried events for).
func (f *Feature) resolveUserID(ctx context.Context, email, tenantID string) (string, error) {
	var userID string
	err := f.db.QueryRowContext(ctx, sqlResolveUserID, email, tenantID).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errUserNotFoundOrNotInTenant
		}
		f.log.Error("Error resolving user: " + err.Error())
		return "", &memberclasserrors.MemberClassError{Code: 500, Message: "error resolving user"}
	}
	return userID, nil
}

// resolveDateRange applies the date-default policy. Mirrors activity_summary.
//   - no dates provided → last 31 days ending now
//   - only startDate    → single-day window [00:00, 23:59:59.999999999]
//   - both provided     → used as-is
func resolveDateRange(req getActivitiesRequest) (time.Time, time.Time) {
	now := time.Now()

	if req.StartDate == nil && req.EndDate == nil {
		return now.AddDate(0, 0, -31), now
	}
	if req.StartDate != nil && req.EndDate == nil {
		s := *req.StartDate
		start := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, s.Location())
		end := time.Date(s.Year(), s.Month(), s.Day(), 23, 59, 59, 999999999, s.Location())
		return start, end
	}
	return *req.StartDate, *req.EndDate
}

func buildPaginationMeta(page, limit int, total int64) dto.PaginationMeta {
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}
	return dto.PaginationMeta{
		Page:        page,
		Limit:       limit,
		TotalCount:  total,
		TotalPages:  totalPages,
		HasNextPage: page < totalPages,
		HasPrevPage: page > 1,
	}
}

func buildCacheKey(tenantID, userID string, req getActivitiesRequest, startDate, endDate time.Time) string {
	startStr, endStr := "", ""
	if req.StartDate != nil {
		startStr = startDate.Format(time.RFC3339)
	}
	if req.EndDate != nil {
		endStr = endDate.Format(time.RFC3339)
	}
	return fmt.Sprintf("user_activities:%s:%s:%d:%d:%s:%s", tenantID, userID, req.Page, req.Limit, startStr, endStr)
}

// ---------- Request parsing + validation ----------

func parseRequest(q url.Values) (*getActivitiesRequest, error) {
	req := &getActivitiesRequest{
		Email: q.Get("email"),
		Page:  1,
		Limit: 10,
	}

	if v := q.Get("page"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("page deve ser um número")
		}
		req.Page = p
	}
	if v := q.Get("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("limit deve ser um número")
		}
		req.Limit = l
	}
	if v := q.Get("startDate"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("formato de data inválido para startDate")
		}
		req.StartDate = &t
	}
	if v := q.Get("endDate"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("formato de data inválido para endDate")
		}
		req.EndDate = &t
	}
	return req, nil
}

func (r *getActivitiesRequest) validate() error {
	if r.Email == "" {
		return errors.New("email é obrigatório")
	}
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}
	if r.EndDate != nil && r.StartDate == nil {
		return errors.New("data de início é obrigatória quando data final é fornecida")
	}
	if r.StartDate != nil && r.EndDate != nil {
		if r.StartDate.After(*r.EndDate) {
			return errors.New("a data de início não pode ser maior que a data de fim")
		}
		if r.EndDate.Sub(*r.StartDate).Hours() > 31*24 {
			return errors.New("período máximo de 31 dias")
		}
	}
	return nil
}

func classifyParseError(err error) string {
	switch err.Error() {
	case "page deve ser um número", "limit deve ser um número":
		return "INVALID_PAGINATION"
	case "formato de data inválido para startDate", "formato de data inválido para endDate":
		return "INVALID_DATE_FORMAT"
	default:
		return "INVALID_REQUEST"
	}
}

func classifyValidationError(err error) string {
	switch err.Error() {
	case "email é obrigatório":
		return "MISSING_EMAIL"
	case "page deve ser >= 1", "limit deve ser entre 1 e 100":
		return "INVALID_PAGINATION"
	case "data de início é obrigatória quando data final é fornecida",
		"a data de início não pode ser maior que a data de fim",
		"período máximo de 31 dias":
		return "INVALID_DATE_RANGE"
	default:
		return "INVALID_REQUEST"
	}
}

// ---------- HTTP helpers ----------

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, errText, message string) {
	writeJSON(w, code, map[string]any{
		"error":   errText,
		"message": message,
	})
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}

// ---------- 3. SQL ----------

func (f *Feature) queryEvents(ctx context.Context, userID, tenantID string, startDate, endDate time.Time, page, limit int) ([]event, error) {
	offset := (page - 1) * limit

	rows, err := f.db.QueryContext(ctx, sqlEventsUnion, userID, tenantID, startDate, endDate, limit, offset)
	if err != nil {
		f.log.Error("Error querying events: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error querying events"}
	}
	defer rows.Close()

	events := make([]event, 0)
	for rows.Next() {
		var (
			id, eventType string
			date          time.Time
			details       []byte
		)
		if err := rows.Scan(&id, &eventType, &date, &details); err != nil {
			f.log.Error("Error scanning event: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error scanning event"}
		}
		events = append(events, event{
			ID:      id,
			Type:    eventType,
			Date:    date,
			Details: append(json.RawMessage(nil), details...),
		})
	}
	if err := rows.Err(); err != nil {
		f.log.Error("Error iterating events: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error iterating events"}
	}
	return events, nil
}

func (f *Feature) countEvents(ctx context.Context, userID, tenantID string, startDate, endDate time.Time) (int64, error) {
	var count int64
	err := f.db.QueryRowContext(ctx, sqlEventsCount, userID, tenantID, startDate, endDate).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		f.log.Error("Error counting events: " + err.Error())
		return 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error counting events"}
	}
	return count, nil
}

const sqlResolveUserID = `
	SELECT uot."userId"
	FROM "UsersOnTenants" uot
	JOIN "User" u ON u.id = uot."userId"
	WHERE u.email = $1
	  AND uot."tenantId" = $2
	LIMIT 1
`

// sqlEventsUnion returns one row per event across 7 sources, projected into
// a common shape (id, type, date, details as jsonb). The outer query sorts
// by date DESC and paginates.
//
// Params: $1 userId, $2 tenantId, $3 startDate, $4 endDate, $5 limit, $6 offset.
const sqlEventsUnion = `
	SELECT id, type, date, details FROM (
		SELECT
			ue.id::text AS id,
			'login' AS type,
			ue."createdAt" AS date,
			jsonb_build_object(
				'whereEvent', ue."whereEvent",
				'withEvent', ue."withEvent",
				'value', ue.value
			) AS details
		FROM "UserEvent" ue
		WHERE ue."usersOnTenantsUserId" = $1
		  AND ue."usersOnTenantsTenantId" = $2
		  AND ue.type = 'login'
		  AND ue."createdAt" BETWEEN $3 AND $4

		UNION ALL

		SELECT
			ue.id::text,
			'acceptTerms',
			ue."createdAt",
			jsonb_build_object(
				'whereEvent', ue."whereEvent",
				'withEvent', ue."withEvent"
			)
		FROM "UserEvent" ue
		WHERE ue."usersOnTenantsUserId" = $1
		  AND ue."usersOnTenantsTenantId" = $2
		  AND ue.type = 'acceptTerms'
		  AND ue."createdAt" BETWEEN $3 AND $4

		UNION ALL

		SELECT
			r.id::text,
			'lessonCompleted',
			r."createdAt",
			jsonb_build_object(
				'lessonId', r."lessonId",
				'lessonName', l.name,
				'rating', r.rating
			)
		FROM "Read" r
		JOIN "Lesson" l ON l.id = r."lessonId"
		JOIN "Module" m ON m.id = l."moduleId"
		JOIN "Course" c ON c.id = m."courseId"
		WHERE r."userId" = $1
		  AND c."tenantId" = $2
		  AND r.read = true
		  AND r."createdAt" BETWEEN $3 AND $4

		UNION ALL

		SELECT
			d.id::text,
			'download',
			d.timestamp,
			jsonb_build_object(
				'arquiveId', d."arquiveId",
				'arquiveName', a.name,
				'size', a.size
			)
		FROM "Download" d
		JOIN "Arquive" a ON a.id = d."arquiveId"
		WHERE d."userId" = $1
		  AND a."tenantId" = $2
		  AND d.timestamp BETWEEN $3 AND $4

		UNION ALL

		SELECT
			cm.id::text,
			'comment',
			cm."createdAt",
			jsonb_build_object(
				'lessonId', cm."lessonId",
				'question', cm.question,
				'answer', cm.answer,
				'published', cm.published
			)
		FROM "Comment" cm
		JOIN "Lesson" l ON l.id = cm."lessonId"
		JOIN "Module" m ON m.id = l."moduleId"
		JOIN "Course" c ON c.id = m."courseId"
		WHERE cm."userId" = $1
		  AND c."tenantId" = $2
		  AND cm."createdAt" BETWEEN $3 AND $4

		UNION ALL

		SELECT
			sq.id::text,
			'quiz',
			sq."createdAt",
			jsonb_build_object(
				'quizId', sq."quizId",
				'quizTitle', q.title,
				'status', sq.status,
				'score', sq.score,
				'attemptOfNum', sq."attemptOfNum"
			)
		FROM "StudentQuiz" sq
		LEFT JOIN "Quiz" q ON q.id = sq."quizId"
		WHERE sq."studentId" = $1
		  AND sq."tenantId" = $2
		  AND sq."deletedAt" IS NULL
		  AND sq."createdAt" BETWEEN $3 AND $4

		UNION ALL

		SELECT
			uc.id::text,
			'certificate',
			uc."createdAt",
			jsonb_build_object(
				'title', uc.title,
				'status', uc.status,
				'certificateUrl', uc."certificateUrl",
				'certificateId', uc.certificate_id
			)
		FROM "UserCertificates" uc
		WHERE uc."userId" = $1
		  AND uc."tenantId" = $2
		  AND uc."createdAt" BETWEEN $3 AND $4
	) events
	ORDER BY date DESC
	LIMIT $5 OFFSET $6
`

// sqlEventsCount is the UNION ALL of the 7 sources with only a 1-column
// projection, wrapped in COUNT(*).
//
// Params: $1 userId, $2 tenantId, $3 startDate, $4 endDate.
const sqlEventsCount = `
	SELECT COUNT(*) FROM (
		SELECT 1 FROM "UserEvent" ue
		WHERE ue."usersOnTenantsUserId" = $1 AND ue."usersOnTenantsTenantId" = $2
		  AND ue.type = 'login' AND ue."createdAt" BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "UserEvent" ue
		WHERE ue."usersOnTenantsUserId" = $1 AND ue."usersOnTenantsTenantId" = $2
		  AND ue.type = 'acceptTerms' AND ue."createdAt" BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "Read" r
		JOIN "Lesson" l ON l.id = r."lessonId"
		JOIN "Module" m ON m.id = l."moduleId"
		JOIN "Course" c ON c.id = m."courseId"
		WHERE r."userId" = $1 AND c."tenantId" = $2 AND r.read = true
		  AND r."createdAt" BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "Download" d
		JOIN "Arquive" a ON a.id = d."arquiveId"
		WHERE d."userId" = $1 AND a."tenantId" = $2
		  AND d.timestamp BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "Comment" cm
		JOIN "Lesson" l ON l.id = cm."lessonId"
		JOIN "Module" m ON m.id = l."moduleId"
		JOIN "Course" c ON c.id = m."courseId"
		WHERE cm."userId" = $1 AND c."tenantId" = $2
		  AND cm."createdAt" BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "StudentQuiz" sq
		WHERE sq."studentId" = $1 AND sq."tenantId" = $2
		  AND sq."deletedAt" IS NULL AND sq."createdAt" BETWEEN $3 AND $4

		UNION ALL
		SELECT 1 FROM "UserCertificates" uc
		WHERE uc."userId" = $1 AND uc."tenantId" = $2
		  AND uc."createdAt" BETWEEN $3 AND $4
	) events
`
