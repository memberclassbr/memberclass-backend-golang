package activity_summary

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

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
)

// ---------- DTOs ----------

type getActivitySummaryRequest struct {
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
	NoAccess  bool
}

type userActivitySummary struct {
	Email        string  `json:"email"`
	UltimoAcesso *string `json:"ultimoAcesso"`
}

type activitySummaryResponse struct {
	Users      []userActivitySummary `json:"users"`
	Pagination dto.PaginationMeta    `json:"pagination"`
}

// ---------- 1. HTTP handler ----------

// GetActivitySummary handles `GET /api/v1/user/activity/summary`.
// Returns paginated users with their last access (or users with no access
// in the given window if `noAccess=true`).
func (f *Feature) GetActivitySummary(w http.ResponseWriter, r *http.Request) {
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

	resp, err := f.getSummary(r.Context(), *req, tenant.ID)
	if err != nil {
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

func (f *Feature) getSummary(ctx context.Context, req getActivitySummaryRequest, tenantID string) (*activitySummaryResponse, error) {
	startDate, endDate := resolveDateRange(req)

	cacheKey := buildCacheKey(tenantID, req, startDate, endDate)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit activitySummaryResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	var (
		users      []userActivitySummary
		totalCount int64
		err        error
	)
	if req.NoAccess {
		users, totalCount, err = f.queryUsersWithoutActivity(ctx, tenantID, startDate, endDate, req.Page, req.Limit)
	} else {
		users, totalCount, err = f.queryUsersWithActivity(ctx, tenantID, startDate, endDate, req.Page, req.Limit)
	}
	if err != nil {
		return nil, err
	}

	resp := &activitySummaryResponse{
		Users:      users,
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

// resolveDateRange applies the slice's date-default policy.
//   - no dates provided → last 31 days ending now
//   - only startDate    → single-day window [00:00, 23:59:59.999999999]
//   - both provided     → used as-is
func resolveDateRange(req getActivitySummaryRequest) (time.Time, time.Time) {
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

func buildCacheKey(tenantID string, req getActivitySummaryRequest, startDate, endDate time.Time) string {
	startStr, endStr := "", ""
	if req.StartDate != nil {
		startStr = startDate.Format(time.RFC3339)
	}
	if req.EndDate != nil {
		endStr = endDate.Format(time.RFC3339)
	}
	noAccess := "false"
	if req.NoAccess {
		noAccess = "true"
	}
	return fmt.Sprintf("activity:summary:%s:%d:%d:%s:%s:%s", tenantID, req.Page, req.Limit, startStr, endStr, noAccess)
}

// ---------- Request parsing + validation ----------

func parseRequest(q url.Values) (*getActivitySummaryRequest, error) {
	req := &getActivitySummaryRequest{Page: 1, Limit: 10}

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
	if q.Get("noAccess") == "true" {
		req.NoAccess = true
	}
	return req, nil
}

func (r *getActivitySummaryRequest) validate() error {
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

// writeError mirrors the legacy `dto.ErrorResponse{Error, Message}` shape
// the existing clients parse.
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

func (f *Feature) queryUsersWithActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]userActivitySummary, int64, error) {
	offset := (page - 1) * limit

	userIDs, err := f.selectUserIDsWithActivity(ctx, tenantID, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	if len(userIDs) == 0 {
		return []userActivitySummary{}, 0, nil
	}

	lastAccessByUser, err := f.selectLastAccess(ctx, tenantID, userIDs, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}

	users, err := f.selectPaginatedUsers(ctx, tenantID, userIDs, limit, offset, lastAccessByUser)
	if err != nil {
		return nil, 0, err
	}

	var totalCount int64
	if err := f.db.QueryRowContext(ctx, sqlCountUsersWithActivity, tenantID, pq.Array(userIDs)).Scan(&totalCount); err != nil {
		f.log.Error("Error counting users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error counting users"}
	}

	return users, totalCount, nil
}

func (f *Feature) selectUserIDsWithActivity(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlUserIDsWithActivity, tenantID, startDate, endDate)
	if err != nil {
		f.log.Error("Error getting users with activity: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error getting users with activity"}
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			f.log.Error("Error scanning user ID: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error scanning user ID"}
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		f.log.Error("Error iterating user IDs: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error iterating user IDs"}
	}
	return ids, nil
}

func (f *Feature) selectLastAccess(ctx context.Context, tenantID string, userIDs []string, startDate, endDate time.Time) (map[string]time.Time, error) {
	rows, err := f.db.QueryContext(ctx, sqlLastAccess, pq.Array(userIDs), tenantID, startDate, endDate)
	if err != nil {
		f.log.Error("Error getting last access: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error getting last access"}
	}
	defer rows.Close()

	byUser := make(map[string]time.Time, len(userIDs))
	for rows.Next() {
		var userID string
		var ts time.Time
		if err := rows.Scan(&userID, &ts); err != nil {
			f.log.Error("Error scanning last access: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error scanning last access"}
		}
		byUser[userID] = ts
	}
	return byUser, nil
}

func (f *Feature) selectPaginatedUsers(ctx context.Context, tenantID string, userIDs []string, limit, offset int, lastAccessByUser map[string]time.Time) ([]userActivitySummary, error) {
	rows, err := f.db.QueryContext(ctx, sqlPaginatedUsers, tenantID, pq.Array(userIDs), limit, offset)
	if err != nil {
		f.log.Error("Error getting paginated users: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error getting paginated users"}
	}
	defer rows.Close()

	var users []userActivitySummary
	for rows.Next() {
		var email, userID string
		if err := rows.Scan(&email, &userID); err != nil {
			f.log.Error("Error scanning user: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error scanning user"}
		}
		var ultimoAcesso *string
		if ts, ok := lastAccessByUser[userID]; ok {
			formatted := ts.Format("2006-01-02T15:04:05.000Z")
			ultimoAcesso = &formatted
		}
		users = append(users, userActivitySummary{Email: email, UltimoAcesso: ultimoAcesso})
	}
	if err := rows.Err(); err != nil {
		f.log.Error("Error iterating users: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error iterating users"}
	}
	return users, nil
}

func (f *Feature) queryUsersWithoutActivity(ctx context.Context, tenantID string, startDate, endDate time.Time, page, limit int) ([]userActivitySummary, int64, error) {
	offset := (page - 1) * limit

	rows, err := f.db.QueryContext(ctx, sqlUsersWithoutActivity, tenantID, startDate, endDate, limit, offset)
	if err != nil {
		f.log.Error("Error getting users without activity: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error getting users without activity"}
	}
	defer rows.Close()

	var users []userActivitySummary
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			f.log.Error("Error scanning user: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error scanning user"}
		}
		users = append(users, userActivitySummary{Email: email, UltimoAcesso: nil})
	}
	if err := rows.Err(); err != nil {
		f.log.Error("Error iterating users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error iterating users"}
	}

	var totalCount int64
	if err := f.db.QueryRowContext(ctx, sqlCountUsersWithoutActivity, tenantID, startDate, endDate).Scan(&totalCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return users, 0, nil
		}
		f.log.Error("Error counting users: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{Code: 500, Message: "error counting users"}
	}

	return users, totalCount, nil
}

const sqlUserIDsWithActivity = `
	SELECT DISTINCT "usersOnTenantsUserId"
	FROM "UserEvent"
	WHERE "usersOnTenantsTenantId" = $1
	  AND "createdAt" >= $2
	  AND "createdAt" <= $3
	  AND "usersOnTenantsUserId" IS NOT NULL
`

const sqlLastAccess = `
	SELECT "usersOnTenantsUserId", MAX("createdAt") as ultimo_acesso
	FROM "UserEvent"
	WHERE "usersOnTenantsUserId" = ANY($1)
	  AND "usersOnTenantsTenantId" = $2
	  AND "createdAt" >= $3
	  AND "createdAt" <= $4
	GROUP BY "usersOnTenantsUserId"
`

const sqlPaginatedUsers = `
	SELECT u.email, uot."userId"
	FROM "UsersOnTenants" uot
	JOIN "User" u ON u.id = uot."userId"
	WHERE uot."tenantId" = $1
	  AND uot."userId" = ANY($2)
	ORDER BY u.email ASC
	LIMIT $3 OFFSET $4
`

const sqlCountUsersWithActivity = `
	SELECT COUNT(*)
	FROM "UsersOnTenants"
	WHERE "tenantId" = $1
	  AND "userId" = ANY($2)
`

const sqlUsersWithoutActivity = `
	SELECT u.email
	FROM "UsersOnTenants" uot
	JOIN "User" u ON u.id = uot."userId"
	WHERE uot."tenantId" = $1
	  AND uot."userId" NOT IN (
	    SELECT DISTINCT "usersOnTenantsUserId"
	    FROM "UserEvent"
	    WHERE "usersOnTenantsTenantId" = $1
	      AND "createdAt" >= $2
	      AND "createdAt" <= $3
	      AND "usersOnTenantsUserId" IS NOT NULL
	  )
	ORDER BY u.email ASC
	LIMIT $4 OFFSET $5
`

const sqlCountUsersWithoutActivity = `
	SELECT COUNT(*)
	FROM "UsersOnTenants" uot
	WHERE uot."tenantId" = $1
	  AND uot."userId" NOT IN (
	    SELECT DISTINCT "usersOnTenantsUserId"
	    FROM "UserEvent"
	    WHERE "usersOnTenantsTenantId" = $1
	      AND "createdAt" >= $2
	      AND "createdAt" <= $3
	      AND "usersOnTenantsUserId" IS NOT NULL
	  )
`
