package student

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeCache struct {
	mu    sync.Mutex
	store map[string]string
	// getOverride lets a test force a hit or an error without seeding.
	getOverride func(key string) (string, error)
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]string{}} }

func (c *fakeCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getOverride != nil {
		return c.getOverride(key)
	}
	v, ok := c.store[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return v, nil
}

func (c *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
	return nil
}

func (c *fakeCache) Increment(context.Context, string, int64) (int64, error) { return 0, nil }
func (c *fakeCache) Delete(context.Context, string) error                    { return nil }
func (c *fakeCache) Exists(context.Context, string) (bool, error)            { return false, nil }
func (c *fakeCache) TTL(context.Context, string) (time.Duration, error)      { return 0, nil }
func (c *fakeCache) Close() error                                            { return nil }

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// ---------- Helpers ----------

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, *fakeCache, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	c := newFakeCache()
	return New(db, c, fakeLogger{}), mock, c, func() { _ = db.Close() }
}

// requestWithTenant builds a GET carrying a tenant in its context, which is
// what the auth middleware does in production.
func requestWithTenant(target, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if tenantID == "" {
		return req
	}
	ctx := context.WithValue(req.Context(), constants.TenantContextKey, &tenant.Tenant{ID: tenantID})
	return req.WithContext(ctx)
}

// expectEnrichment queues the four follow-up queries the report runs once it
// has at least one student.
func expectEnrichment(mock sqlmock.Sqlmock, tenantID string) {
	mock.ExpectQuery(`FROM "Delivery"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("d1", "Curso Base"))

	mock.ExpectQuery(`FROM "MemberOnDelivery"`).
		WillReturnRows(sqlmock.NewRows([]string{"memberId", "deliveryId"}).AddRow("u1", "d1"))

	mock.ExpectQuery(`FROM "Read"`).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"}).
			AddRow("u1", "l1", "Aula 1", time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))

	mock.ExpectQuery(`FROM "UserEvent"`).
		WillReturnRows(sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"}).
			AddRow("u1", time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)))
}

// ---------- 1. Request parsing / validation ----------

func TestParseRequest(t *testing.T) {
	t.Run("defaults when no query", func(t *testing.T) {
		req, err := parseRequest(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, req.Page)
		assert.Equal(t, 10, req.Limit)
		assert.Nil(t, req.StartDate)
		assert.Nil(t, req.EndDate)
	})

	t.Run("non-numeric page is a pagination error", func(t *testing.T) {
		_, err := parseRequest(map[string][]string{"page": {"abc"}})
		require.Error(t, err)
		assert.Equal(t, "INVALID_PAGINATION", parseErrorCode(err))
	})

	t.Run("bad date format is a date error", func(t *testing.T) {
		for _, field := range []string{"startDate", "endDate"} {
			_, err := parseRequest(map[string][]string{field: {"01/02/2026"}})
			require.Error(t, err, field)
			assert.Equal(t, "INVALID_DATE_FORMAT", parseErrorCode(err), field)
		}
	})
}

func TestValidate(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		req      getStudentReportRequest
		wantErr  string
		wantCode string
	}{
		{"valid", getStudentReportRequest{Page: 1, Limit: 10}, "", ""},
		{"page below one", getStudentReportRequest{Page: 0, Limit: 10}, errPageRange, "INVALID_PAGINATION"},
		{"limit below one", getStudentReportRequest{Page: 1, Limit: 0}, errLimitRange, "INVALID_PAGINATION"},
		{"limit above hundred", getStudentReportRequest{Page: 1, Limit: 101}, errLimitRange, "INVALID_PAGINATION"},
		{"end without start", getStudentReportRequest{Page: 1, Limit: 10, EndDate: &feb}, errStartRequired, "INVALID_REQUEST"},
		{"start after end", getStudentReportRequest{Page: 1, Limit: 10, StartDate: &feb, EndDate: &jan}, errStartAfterEnd, "INVALID_DATE_RANGE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr)
			_, code := validationErrorResponse(err)
			assert.Equal(t, tc.wantCode, code)
		})
	}
}

// ---------- 2. Handler ----------

func TestGetStudentReport_RejectsNonGet(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetStudentReport(w, httptest.NewRequest(http.MethodPost, "/report", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// Without a tenant in context there is no scope for the query, so the request
// must be rejected before any SQL runs.
func TestGetStudentReport_RequiresTenant(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetStudentReport(w, requestWithTenant("/report", ""))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_API_KEY", errorCodeOf(t, w))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetStudentReport_ValidationErrorsCarryCodes(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantCode string
	}{
		{"page zero", "?page=0", "INVALID_PAGINATION"},
		{"limit too large", "?limit=500", "INVALID_PAGINATION"},
		{"unparsable date", "?startDate=nope", "INVALID_DATE_FORMAT"},
		{"inverted range", "?startDate=2026-02-01T00:00:00Z&endDate=2026-01-01T00:00:00Z", "INVALID_DATE_RANGE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.GetStudentReport(w, requestWithTenant("/report"+tc.query, "t1"))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.wantCode, errorCodeOf(t, w))
		})
	}
}

func TestGetStudentReport_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "UsersOnTenants" uot`).
		WithArgs("t1", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
			AddRow("u1", "aluno@example.com", "12345678900", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)))
	expectEnrichment(mock, "t1")
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM "UsersOnTenants"`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	w := httptest.NewRecorder()
	f.GetStudentReport(w, requestWithTenant("/report", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp studentReportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Alunos, 1)
	student := resp.Alunos[0]
	assert.Equal(t, "u1", student.AlunoIDMemberClass)
	assert.Equal(t, "aluno@example.com", student.Email)
	assert.Equal(t, "12345678900", student.Cpf)
	assert.Equal(t, []string{"Curso Base"}, student.EntregasVinculadas)
	assert.Equal(t, 1, student.QuantidadeAulasAssistidas)
	require.Len(t, student.AulasAssistidas, 1)
	assert.Equal(t, "Aula 1", student.AulasAssistidas[0].Titulo)
	require.NotNil(t, student.UltimoAcesso)
	assert.Equal(t, "2026-04-01T09:00:00Z", *student.UltimoAcesso)

	assert.Equal(t, int64(1), resp.Pagination.TotalCount)
	assert.Equal(t, 1, resp.Pagination.TotalPages)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A page with no students must not run the enrichment or count queries; the
// mock would fail on an unexpected call if it did.
func TestGetStudentReport_EmptyPageSkipsEnrichment(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "UsersOnTenants" uot`).
		WithArgs("t1", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}))

	w := httptest.NewRecorder()
	f.GetStudentReport(w, requestWithTenant("/report", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp studentReportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Alunos)
	assert.Equal(t, int64(0), resp.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetStudentReport_QueryFailureIsFiveHundred(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "UsersOnTenants" uot`).
		WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GetStudentReport(w, requestWithTenant("/report", "t1"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// The driver error must not reach the client.
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// ---------- 3. Rules ----------

func TestGetReport_ServesFromCacheWithoutQuerying(t *testing.T) {
	f, mock, c, done := newTestFeature(t)
	defer done()

	cached, err := json.Marshal(studentReportResponse{
		Alunos: []studentReport{{AlunoIDMemberClass: "cached-user"}},
	})
	require.NoError(t, err)
	c.getOverride = func(string) (string, error) { return string(cached), nil }

	resp, err := f.getReport(context.Background(), getStudentReportRequest{Page: 1, Limit: 10}, "t1")
	require.NoError(t, err)
	require.Len(t, resp.Alunos, 1)
	assert.Equal(t, "cached-user", resp.Alunos[0].AlunoIDMemberClass)
	// No ExpectQuery was queued: reaching the database here would fail.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The cache key must include the tenant. A shared key would serve one tenant's
// roster to another.
func TestBuildCacheKey_VariesByTenantAndPage(t *testing.T) {
	base := getStudentReportRequest{Page: 1, Limit: 10}

	keyA := buildCacheKey("tenant-a", base)
	keyB := buildCacheKey("tenant-b", base)
	assert.NotEqual(t, keyA, keyB, "different tenants must not share a cache entry")

	page2 := base
	page2.Page = 2
	assert.NotEqual(t, keyA, buildCacheKey("tenant-a", page2))

	assert.Equal(t, keyA, buildCacheKey("tenant-a", base), "same input must be stable")
}

func TestBuildCacheKey_VariesByDateRange(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	base := getStudentReportRequest{Page: 1, Limit: 10}
	withRange := getStudentReportRequest{Page: 1, Limit: 10, StartDate: &start, EndDate: &end}

	assert.NotEqual(t, buildCacheKey("t1", base), buildCacheKey("t1", withRange))
}

// ---------- 4. SQL ----------

// The date bounds are optional and numbered after the tenant, so their
// placeholders must shift the LIMIT/OFFSET ones.
func TestQueryStudents_DateFilterShiftsPlaceholders(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`LIMIT \$4 OFFSET \$5`).
		WithArgs("t1", start, end, 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}))

	_, err := f.queryStudents(context.Background(), "t1", getStudentReportRequest{
		Page: 2, Limit: 20, StartDate: &start, EndDate: &end,
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The report is tenant-scoped; every query must carry the tenant id.
func TestQueries_AreTenantScoped(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Delivery" WHERE "tenantId" = \$1`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectQuery(`FROM "MemberOnDelivery"`).
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnRows(sqlmock.NewRows([]string{"memberId", "deliveryId"}))
	mock.ExpectQuery(`v\."tenantId" = \$2`).
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnRows(sqlmock.NewRows([]string{"userId", "lessonId", "lesson_name", "createdAt"}))
	mock.ExpectQuery(`"usersOnTenantsTenantId" = \$2`).
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnRows(sqlmock.NewRows([]string{"usersOnTenantsUserId", "createdAt"}))

	ctx := context.Background()
	_, err := f.queryDeliveryNames(ctx, "t1")
	require.NoError(t, err)
	_, err = f.queryUserDeliveries(ctx, []string{"u1"}, "t1")
	require.NoError(t, err)
	_, err = f.queryLessonsWatched(ctx, []string{"u1"}, "t1")
	require.NoError(t, err)
	_, err = f.queryLastAccesses(ctx, []string{"u1"}, "t1")
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountStudents_NoRowsMeansZero(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(sql.ErrNoRows)

	total, err := f.countStudents(context.Background(), "t1", getStudentReportRequest{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
}

// Duplicate rows for one user collapse into a single entry, and the SQL
// ordering is preserved in the response.
func TestQueryStudents_DedupesAndKeepsOrder(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	newest := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM "UsersOnTenants" uot`).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "cpf", "assignedAt"}).
			AddRow("u2", "b@example.com", "", newest).
			AddRow("u2", "b@example.com", "", newest).
			AddRow("u1", "a@example.com", "", older))

	students, err := f.queryStudents(context.Background(), "t1", getStudentReportRequest{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, students, 2)
	assert.Equal(t, "u2", students[0].AlunoIDMemberClass)
	assert.Equal(t, "u1", students[1].AlunoIDMemberClass)
}

// ---------- helpers ----------

func errorCodeOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		ErrorCode string `json:"errorCode"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.ErrorCode
}
