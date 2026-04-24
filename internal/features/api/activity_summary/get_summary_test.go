package activity_summary

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
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes (replace mockery-generated mocks) ----------

type fakeCache struct {
	mu    sync.Mutex
	store map[string]string
	// Overrides let each test force specific return values / errors.
	getOverride func(key string) (string, error)
	setOverride func(key, value string) error
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]string{}} }

func (c *fakeCache) Get(ctx context.Context, key string) (string, error) {
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
func (c *fakeCache) Set(ctx context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setOverride != nil {
		return c.setOverride(key, value)
	}
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
	cache := newFakeCache()
	f := New(db, cache, fakeLogger{})
	return f, mock, cache, func() { _ = db.Close() }
}

// ---------- 1. Request parsing / validation ----------

func TestParseRequest(t *testing.T) {
	t.Run("defaults when no query", func(t *testing.T) {
		req, err := parseRequest(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, req.Page)
		assert.Equal(t, 10, req.Limit)
		assert.False(t, req.NoAccess)
		assert.Nil(t, req.StartDate)
		assert.Nil(t, req.EndDate)
	})

	t.Run("invalid page returns error with correct code", func(t *testing.T) {
		_, err := parseRequest(map[string][]string{"page": {"abc"}})
		require.Error(t, err)
		assert.Equal(t, "INVALID_PAGINATION", classifyParseError(err))
	})

	t.Run("invalid date format", func(t *testing.T) {
		_, err := parseRequest(map[string][]string{"startDate": {"not-a-date"}})
		require.Error(t, err)
		assert.Equal(t, "INVALID_DATE_FORMAT", classifyParseError(err))
	})
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		req     getActivitySummaryRequest
		wantErr string
		wantCls string
	}{
		{"page < 1", getActivitySummaryRequest{Page: 0, Limit: 10}, "page deve ser >= 1", "INVALID_PAGINATION"},
		{"limit > 100", getActivitySummaryRequest{Page: 1, Limit: 101}, "limit deve ser entre 1 e 100", "INVALID_PAGINATION"},
		{"endDate without startDate", getActivitySummaryRequest{Page: 1, Limit: 10, EndDate: ptr(time.Now())}, "data de início é obrigatória quando data final é fornecida", "INVALID_DATE_RANGE"},
		{"startDate after endDate", getActivitySummaryRequest{Page: 1, Limit: 10, StartDate: ptr(time.Now()), EndDate: ptr(time.Now().AddDate(0, 0, -1))}, "a data de início não pode ser maior que a data de fim", "INVALID_DATE_RANGE"},
		{"period > 31 days", getActivitySummaryRequest{Page: 1, Limit: 10, StartDate: ptr(time.Now().AddDate(0, 0, -40)), EndDate: ptr(time.Now())}, "período máximo de 31 dias", "INVALID_DATE_RANGE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.validate()
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, err.Error())
			assert.Equal(t, tc.wantCls, classifyValidationError(err))
		})
	}
}

// ---------- 2. Business rule (getSummary) ----------

func TestGetSummary_CacheHit(t *testing.T) {
	f, _, cache, done := newTestFeature(t)
	defer done()

	tenantID := "tenant-123"
	req := getActivitySummaryRequest{Page: 1, Limit: 10}

	lastAccess := "2024-01-15T10:30:00.000Z"
	cached := activitySummaryResponse{
		Users:      []userActivitySummary{{Email: "a@b.com", UltimoAcesso: &lastAccess}},
		Pagination: dto.PaginationMeta{Page: 1, Limit: 10, TotalCount: 1, TotalPages: 1},
	}
	raw, _ := json.Marshal(cached)
	cache.store[buildCacheKey(tenantID, req, time.Time{}, time.Time{})] = string(raw)

	got, err := f.getSummary(context.Background(), req, tenantID)
	require.NoError(t, err)
	assert.Len(t, got.Users, 1)
	assert.Equal(t, "a@b.com", got.Users[0].Email)
}

func TestGetSummary_WithActivity_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	tenantID := "tenant-123"
	req := getActivitySummaryRequest{Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT DISTINCT "usersOnTenantsUserId"`).
		WillReturnRows(sqlmock.NewRows([]string{"usersOnTenantsUserId"}).AddRow("u-1").AddRow("u-2"))

	lastAccess := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT "usersOnTenantsUserId", MAX\("createdAt"\)`).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "ultimo_acesso"}).AddRow("u-1", lastAccess))

	mock.ExpectQuery(`SELECT u.email, uot."userId"`).
		WillReturnRows(sqlmock.NewRows([]string{"email", "userId"}).
			AddRow("a@b.com", "u-1").
			AddRow("c@d.com", "u-2"))

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM "UsersOnTenants"\s+WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	got, err := f.getSummary(context.Background(), req, tenantID)
	require.NoError(t, err)
	assert.Len(t, got.Users, 2)
	require.NotNil(t, got.Users[0].UltimoAcesso)
	assert.Equal(t, "2024-01-15T10:30:00.000Z", *got.Users[0].UltimoAcesso)
	assert.Nil(t, got.Users[1].UltimoAcesso)
	assert.Equal(t, int64(2), got.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSummary_WithActivity_NoUsersFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	tenantID := "tenant-123"
	req := getActivitySummaryRequest{Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT DISTINCT "usersOnTenantsUserId"`).
		WillReturnRows(sqlmock.NewRows([]string{"usersOnTenantsUserId"}))

	got, err := f.getSummary(context.Background(), req, tenantID)
	require.NoError(t, err)
	assert.Empty(t, got.Users)
	assert.Equal(t, int64(0), got.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSummary_NoAccess(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	tenantID := "tenant-123"
	req := getActivitySummaryRequest{Page: 1, Limit: 10, NoAccess: true}

	mock.ExpectQuery(`SELECT u.email\s+FROM "UsersOnTenants"`).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("nobody@x.com"))

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM "UsersOnTenants" uot\s+WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	got, err := f.getSummary(context.Background(), req, tenantID)
	require.NoError(t, err)
	require.Len(t, got.Users, 1)
	assert.Nil(t, got.Users[0].UltimoAcesso)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSummary_RepoError(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT DISTINCT "usersOnTenantsUserId"`).
		WillReturnError(errors.New("database connection error"))

	_, err := f.getSummary(context.Background(), getActivitySummaryRequest{Page: 1, Limit: 10}, "tenant-123")
	require.Error(t, err)
	var mcErr *memberclasserrors.MemberClassError
	require.True(t, errors.As(err, &mcErr))
	assert.Equal(t, 500, mcErr.Code)
}

func TestGetSummary_PaginationMath(t *testing.T) {
	meta := buildPaginationMeta(2, 10, 25)
	assert.Equal(t, 2, meta.Page)
	assert.Equal(t, 10, meta.Limit)
	assert.Equal(t, int64(25), meta.TotalCount)
	assert.Equal(t, 3, meta.TotalPages)
	assert.True(t, meta.HasNextPage)
	assert.True(t, meta.HasPrevPage)
}

// ---------- 3. HTTP handler ----------

func TestGetActivitySummary_MethodNotAllowed(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetActivitySummary(w, httptest.NewRequest(http.MethodPost, "/api/v1/user/activity/summary", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestGetActivitySummary_TenantMissing(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetActivitySummary(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/activity/summary", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["ok"])
	assert.Equal(t, "INVALID_API_KEY", body["errorCode"])
}

func TestGetActivitySummary_InvalidPagination(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activity/summary?page=abc", nil))
	w := httptest.NewRecorder()
	f.GetActivitySummary(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_PAGINATION", body["errorCode"])
}

func TestGetActivitySummary_InvalidDateRange(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activity/summary?endDate=2024-01-01T00:00:00Z", nil))
	w := httptest.NewRecorder()
	f.GetActivitySummary(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_DATE_RANGE", body["errorCode"])
}

func TestGetActivitySummary_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT DISTINCT "usersOnTenantsUserId"`).
		WillReturnRows(sqlmock.NewRows([]string{"usersOnTenantsUserId"}).AddRow("u-1"))
	mock.ExpectQuery(`SELECT "usersOnTenantsUserId", MAX\("createdAt"\)`).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "ultimo_acesso"}))
	mock.ExpectQuery(`SELECT u.email, uot."userId"`).
		WillReturnRows(sqlmock.NewRows([]string{"email", "userId"}).AddRow("a@b.com", "u-1"))
	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM "UsersOnTenants"\s+WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activity/summary?page=1&limit=10", nil))
	w := httptest.NewRecorder()
	f.GetActivitySummary(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body activitySummaryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Users, 1)
	assert.Equal(t, "a@b.com", body.Users[0].Email)
}

// ---------- misc ----------

func withTenant(r *http.Request) *http.Request {
	t := &tenant.Tenant{ID: "tenant-123"}
	ctx := context.WithValue(r.Context(), constants.TenantContextKey, t)
	return r.WithContext(ctx)
}

func ptr[T any](v T) *T { return &v }

// Ensure we compile against *sql.DB (sanity check for New signature).
var _ = func() *Feature { return New((*sql.DB)(nil), (*fakeCache)(nil), fakeLogger{}) }
