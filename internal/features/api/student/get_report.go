package student

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/pagination"
)

// reportCacheTTL matches the previous implementation. The report fans out to
// six queries, so it is worth caching even briefly.
const reportCacheTTL = 300 * time.Second

// ---------- DTOs ----------

type getStudentReportRequest struct {
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
}

type lessonWatched struct {
	AulaID        string `json:"aula_id"`
	Titulo        string `json:"titulo"`
	DataAssistida string `json:"data_assistida"`
}

type studentReport struct {
	AlunoIDMemberClass        string          `json:"aluno_id_member_class"`
	Email                     string          `json:"email"`
	Cpf                       string          `json:"cpf"`
	DataCadastro              string          `json:"data_cadastro"`
	EntregasVinculadas        []string        `json:"entregas_vinculadas"`
	UltimoAcesso              *string         `json:"ultimo_acesso"`
	QuantidadeAulasAssistidas int             `json:"quantidade_aulas_assistidas"`
	AulasAssistidas           []lessonWatched `json:"aulas_assistidas"`
}

type studentReportResponse struct {
	Alunos     []studentReport `json:"alunos"`
	Pagination pagination.Meta `json:"pagination"`
}

// ---------- 1. HTTP handler ----------

// GetStudentReport handles `GET /api/v1/student/report`.
func (f *Feature) GetStudentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req, err := parseRequest(r.URL.Query())
	if err != nil {
		writeCustomError(w, http.StatusBadRequest, err.Error(), parseErrorCode(err))
		return
	}

	if err := req.validate(); err != nil {
		message, code := validationErrorResponse(err)
		writeCustomError(w, http.StatusBadRequest, message, code)
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	resp, err := f.getReport(r.Context(), *req, tenant.ID)
	if err != nil {
		var mcErr *memberclasserrors.MemberClassError
		if errors.As(err, &mcErr) {
			writeError(w, mcErr.Code, mcErr.Message)
			return
		}
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseRequest(query url.Values) (*getStudentReportRequest, error) {
	req := &getStudentReportRequest{Page: 1, Limit: 10}

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

	if v := query.Get("startDate"); v != "" {
		startDate, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New(errInvalidStartDate)
		}
		req.StartDate = &startDate
	}

	if v := query.Get("endDate"); v != "" {
		endDate, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New(errInvalidEndDate)
		}
		req.EndDate = &endDate
	}

	return req, nil
}

const (
	errInvalidStartDate = "formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)"
	errInvalidEndDate   = "formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)"
	errPageRange        = "page deve ser >= 1"
	errLimitRange       = "limit deve ser entre 1 e 100"
	errStartAfterEnd    = "a data de início não pode ser maior que a data de fim"
	errStartRequired    = "data de início é obrigatória quando data final é fornecida"
)

func (r *getStudentReportRequest) validate() error {
	if r.Page < 1 {
		return errors.New(errPageRange)
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New(errLimitRange)
	}
	if r.EndDate != nil && r.StartDate == nil {
		return errors.New(errStartRequired)
	}
	if r.StartDate != nil && r.EndDate != nil && r.StartDate.After(*r.EndDate) {
		return errors.New(errStartAfterEnd)
	}
	return nil
}

// parseErrorCode maps a parse failure to the error code clients switch on.
// Only the two date formats are distinguishable; everything else that can fail
// during parsing is a pagination field.
func parseErrorCode(err error) string {
	switch err.Error() {
	case errInvalidStartDate, errInvalidEndDate:
		return "INVALID_DATE_FORMAT"
	default:
		return "INVALID_PAGINATION"
	}
}

// validationErrorResponse maps a validation failure to the message and code the
// client sees. The messages differ from the internal error strings — the API
// returns a combined sentence for pagination and a capitalised one for dates.
func validationErrorResponse(err error) (message, code string) {
	switch err.Error() {
	case errPageRange, errLimitRange:
		return "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100", "INVALID_PAGINATION"
	case errInvalidStartDate:
		return "Formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)", "INVALID_DATE_FORMAT"
	case errInvalidEndDate:
		return "Formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)", "INVALID_DATE_FORMAT"
	case errStartAfterEnd:
		return "startDate não pode ser maior que endDate", "INVALID_DATE_RANGE"
	default:
		return err.Error(), "INVALID_REQUEST"
	}
}

// ---------- 2. Business rule ----------

func (f *Feature) getReport(ctx context.Context, req getStudentReportRequest, tenantID string) (*studentReportResponse, error) {
	cacheKey := buildCacheKey(tenantID, req)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit studentReportResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	students, err := f.queryStudents(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	// No students on this page means nothing to enrich and, matching the
	// previous behaviour, a zeroed pagination envelope rather than a count
	// query.
	if len(students) == 0 {
		return &studentReportResponse{
			Alunos:     []studentReport{},
			Pagination: pagination.NewMeta(req.Page, req.Limit, 0),
		}, nil
	}

	userIDs := make([]string, 0, len(students))
	for i := range students {
		userIDs = append(userIDs, students[i].AlunoIDMemberClass)
	}

	deliveryNames, err := f.queryDeliveryNames(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	deliveriesByUser, err := f.queryUserDeliveries(ctx, userIDs, tenantID)
	if err != nil {
		return nil, err
	}
	lessonsByUser, err := f.queryLessonsWatched(ctx, userIDs, tenantID)
	if err != nil {
		return nil, err
	}
	lastAccessByUser, err := f.queryLastAccesses(ctx, userIDs, tenantID)
	if err != nil {
		return nil, err
	}

	for i := range students {
		s := &students[i]

		names := []string{}
		for _, deliveryID := range deliveriesByUser[s.AlunoIDMemberClass] {
			name, ok := deliveryNames[deliveryID]
			if ok && !contains(names, name) {
				names = append(names, name)
			}
		}
		s.EntregasVinculadas = names

		if lessons, ok := lessonsByUser[s.AlunoIDMemberClass]; ok {
			s.AulasAssistidas = lessons
			s.QuantidadeAulasAssistidas = len(lessons)
		}

		if lastAccess, ok := lastAccessByUser[s.AlunoIDMemberClass]; ok {
			formatted := lastAccess.Format(time.RFC3339)
			s.UltimoAcesso = &formatted
		}
	}

	totalCount, err := f.countStudents(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	resp := &studentReportResponse{
		Alunos:     students,
		Pagination: pagination.NewMeta(req.Page, req.Limit, totalCount),
	}

	if encoded, err := json.Marshal(resp); err == nil {
		if err := f.cache.Set(ctx, cacheKey, string(encoded), reportCacheTTL); err != nil {
			f.log.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			f.log.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return resp, nil
}

func buildCacheKey(tenantID string, req getStudentReportRequest) string {
	payload := map[string]interface{}{
		"tenantId":  tenantID,
		"page":      req.Page,
		"limit":     req.Limit,
		"startDate": nil,
		"endDate":   nil,
	}
	if req.StartDate != nil {
		payload["startDate"] = req.StartDate.Format(time.RFC3339)
	}
	if req.EndDate != nil {
		payload["endDate"] = req.EndDate.Format(time.RFC3339)
	}

	encoded, _ := json.Marshal(payload)
	hash := md5.Sum(encoded)
	return fmt.Sprintf("alunos_relatorio:%s", hex.EncodeToString(hash[:]))
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ---------- 3. SQL ----------

const (
	// sqlStudents lists the tenant's students for one page. The date filters
	// are appended by dateFilter below, which is why the ORDER BY / LIMIT tail
	// is a separate constant.
	sqlStudents = `
		SELECT
			uot."userId",
			u.email,
			COALESCE(uot.document, '') as cpf,
			uot."assignedAt"
		FROM "UsersOnTenants" uot
		JOIN "User" u ON uot."userId" = u.id
		WHERE uot."tenantId" = $1`

	sqlStudentsTail = ` ORDER BY uot."assignedAt" DESC LIMIT $%d OFFSET $%d`

	sqlCountStudents = `SELECT COUNT(*) FROM "UsersOnTenants" WHERE "tenantId" = $1`

	sqlDeliveries = `SELECT id, name FROM "Delivery" WHERE "tenantId" = $1`

	sqlUserDeliveries = `SELECT "memberId", "deliveryId" FROM "MemberOnDelivery" WHERE "memberId" = ANY($1) AND "tenantId" = $2`

	// sqlLessonsWatched walks Lesson up to the Vitrine so the tenant filter is
	// applied at the only place that carries a tenantId.
	sqlLessonsWatched = `
		SELECT
			r."userId",
			r."lessonId",
			l.name as lesson_name,
			r."createdAt"
		FROM "Read" r
		JOIN "Lesson" l ON r."lessonId" = l.id
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE r."userId" = ANY($1)
		  AND v."tenantId" = $2
		ORDER BY r."createdAt" DESC
	`

	sqlLastAccesses = `
		SELECT DISTINCT ON ("usersOnTenantsUserId")
			"usersOnTenantsUserId",
			"createdAt"
		FROM "UserEvent"
		WHERE "usersOnTenantsUserId" = ANY($1)
		  AND "usersOnTenantsTenantId" = $2
		  AND type = 'login'
		ORDER BY "usersOnTenantsUserId", "createdAt" DESC
	`
)

// dateFilter appends the optional assignedAt bounds, using the given column
// reference so the same logic serves both the page query (aliased table) and
// the count query (bare column).
func dateFilter(column string, req getStudentReportRequest, args []interface{}) (string, []interface{}) {
	clause := ""
	if req.StartDate != nil {
		clause += fmt.Sprintf(` AND %s >= $%d`, column, len(args)+1)
		args = append(args, *req.StartDate)
	}
	if req.EndDate != nil {
		clause += fmt.Sprintf(` AND %s <= $%d`, column, len(args)+1)
		args = append(args, *req.EndDate)
	}
	return clause, args
}

func (f *Feature) queryStudents(ctx context.Context, tenantID string, req getStudentReportRequest) ([]studentReport, error) {
	args := []interface{}{tenantID}
	clause, args := dateFilter(`uot."assignedAt"`, req, args)

	query := sqlStudents + clause + fmt.Sprintf(sqlStudentsTail, len(args)+1, len(args)+2)
	args = append(args, req.Limit, pagination.Offset(req.Page, req.Limit))

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, f.fail("Error getting students report: ", err, "error getting students report")
	}
	defer rows.Close()

	students := make([]studentReport, 0)
	seen := make(map[string]bool)

	for rows.Next() {
		var userID, email, cpf string
		var assignedAt time.Time

		if err := rows.Scan(&userID, &email, &cpf, &assignedAt); err != nil {
			return nil, f.fail("Error scanning student: ", err, "error scanning student")
		}
		if seen[userID] {
			continue
		}
		seen[userID] = true

		students = append(students, studentReport{
			AlunoIDMemberClass:        userID,
			Email:                     email,
			Cpf:                       cpf,
			DataCadastro:              assignedAt.Format(time.RFC3339),
			EntregasVinculadas:        []string{},
			UltimoAcesso:              nil,
			QuantidadeAulasAssistidas: 0,
			AulasAssistidas:           []lessonWatched{},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, f.fail("Error iterating students: ", err, "error iterating students")
	}

	return students, nil
}

func (f *Feature) countStudents(ctx context.Context, tenantID string, req getStudentReportRequest) (int64, error) {
	args := []interface{}{tenantID}
	clause, args := dateFilter(`"assignedAt"`, req, args)

	var total int64
	err := f.db.QueryRowContext(ctx, sqlCountStudents+clause, args...).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, f.fail("Error counting students: ", err, "error counting students")
	}
	return total, nil
}

func (f *Feature) queryDeliveryNames(ctx context.Context, tenantID string) (map[string]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlDeliveries, tenantID)
	if err != nil {
		return nil, f.fail("Error getting deliveries: ", err, "error getting deliveries")
	}
	defer rows.Close()

	names := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, f.fail("Error scanning delivery: ", err, "error scanning delivery")
		}
		names[id] = name
	}
	return names, nil
}

func (f *Feature) queryUserDeliveries(ctx context.Context, userIDs []string, tenantID string) (map[string][]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlUserDeliveries, pq.Array(userIDs), tenantID)
	if err != nil {
		return nil, f.fail("Error getting member deliveries: ", err, "error getting member deliveries")
	}
	defer rows.Close()

	byUser := make(map[string][]string)
	for rows.Next() {
		var memberID, deliveryID string
		if err := rows.Scan(&memberID, &deliveryID); err != nil {
			return nil, f.fail("Error scanning member delivery: ", err, "error scanning member delivery")
		}
		byUser[memberID] = append(byUser[memberID], deliveryID)
	}
	return byUser, nil
}

func (f *Feature) queryLessonsWatched(ctx context.Context, userIDs []string, tenantID string) (map[string][]lessonWatched, error) {
	rows, err := f.db.QueryContext(ctx, sqlLessonsWatched, pq.Array(userIDs), tenantID)
	if err != nil {
		return nil, f.fail("Error getting lessons watched: ", err, "error getting lessons watched")
	}
	defer rows.Close()

	byUser := make(map[string][]lessonWatched)
	for rows.Next() {
		var userID, lessonID, lessonName string
		var createdAt time.Time

		if err := rows.Scan(&userID, &lessonID, &lessonName, &createdAt); err != nil {
			return nil, f.fail("Error scanning lesson: ", err, "error scanning lesson")
		}
		byUser[userID] = append(byUser[userID], lessonWatched{
			AulaID:        lessonID,
			Titulo:        lessonName,
			DataAssistida: createdAt.Format(time.RFC3339),
		})
	}
	return byUser, nil
}

func (f *Feature) queryLastAccesses(ctx context.Context, userIDs []string, tenantID string) (map[string]time.Time, error) {
	rows, err := f.db.QueryContext(ctx, sqlLastAccesses, pq.Array(userIDs), tenantID)
	if err != nil {
		return nil, f.fail("Error getting last accesses: ", err, "error getting last accesses")
	}
	defer rows.Close()

	byUser := make(map[string]time.Time)
	for rows.Next() {
		var userID string
		var createdAt time.Time

		if err := rows.Scan(&userID, &createdAt); err != nil {
			return nil, f.fail("Error scanning last access: ", err, "error scanning last access")
		}
		if _, exists := byUser[userID]; !exists {
			byUser[userID] = createdAt
		}
	}
	return byUser, nil
}

// fail logs the underlying error and returns the 500 the client sees. Database
// details stay in the log.
func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

// ---------- responses ----------

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeCustomError(w, code, message, "INTERNAL_ERROR")
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
