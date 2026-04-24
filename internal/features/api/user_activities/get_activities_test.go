package user_activities

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

// ---------- Local fakes (no mockery) ----------

type fakeCache struct {
	mu    sync.Mutex
	store map[string]string
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]string{}} }

func (c *fakeCache) Get(ctx context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.store[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return v, nil
}
func (c *fakeCache) Set(ctx context.Context, key, value string, _ time.Duration) error {
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

// newTestFeature builds a Feature in dev mode (cache bypassed) so tests don't
// exercise the cache path unless they opt in by setting f.devMode = false.
func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, *fakeCache, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	cache := newFakeCache()
	f := New(db, cache, fakeLogger{})
	f.devMode = true
	return f, mock, cache, func() { _ = db.Close() }
}

func withTenant(r *http.Request) *http.Request {
	t := &tenant.Tenant{ID: "tenant-123"}
	ctx := context.WithValue(r.Context(), constants.TenantContextKey, t)
	return r.WithContext(ctx)
}

func ptr[T any](v T) *T { return &v }

// expectResolveUserID matches the SELECT uot."userId" resolver query.
func expectResolveUserID(mock sqlmock.Sqlmock, email, tenantID, userID string) {
	mock.ExpectQuery(`SELECT uot."userId"`).
		WithArgs(email, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(userID))
}

// expectEventsAndCount stubs the UNION + COUNT queries with the provided rows/total.
func expectEventsAndCount(mock sqlmock.Sqlmock, eventRows *sqlmock.Rows, total int64) {
	mock.ExpectQuery(`SELECT id, type, date, details FROM \(`).WillReturnRows(eventRows)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
}

// ---------- 1. Request parsing / validation ----------

func TestParseRequest_Defaults(t *testing.T) {
	req, err := parseRequest(map[string][]string{"email": {"a@b.com"}})
	require.NoError(t, err)
	assert.Equal(t, "a@b.com", req.Email)
	assert.Equal(t, 1, req.Page)
	assert.Equal(t, 10, req.Limit)
	assert.Nil(t, req.StartDate)
	assert.Nil(t, req.EndDate)
}

func TestParseRequest_InvalidPage(t *testing.T) {
	_, err := parseRequest(map[string][]string{"email": {"a@b.com"}, "page": {"abc"}})
	require.Error(t, err)
	assert.Equal(t, "INVALID_PAGINATION", classifyParseError(err))
}

func TestParseRequest_InvalidDate(t *testing.T) {
	_, err := parseRequest(map[string][]string{"email": {"a@b.com"}, "startDate": {"zzz"}})
	require.Error(t, err)
	assert.Equal(t, "INVALID_DATE_FORMAT", classifyParseError(err))
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name, wantErr, wantCls string
		req                    getActivitiesRequest
	}{
		{"missing email", "email é obrigatório", "MISSING_EMAIL", getActivitiesRequest{Email: "", Page: 1, Limit: 10}},
		{"page < 1", "page deve ser >= 1", "INVALID_PAGINATION", getActivitiesRequest{Email: "a@b.com", Page: 0, Limit: 10}},
		{"limit > 100", "limit deve ser entre 1 e 100", "INVALID_PAGINATION", getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 101}},
		{"end without start", "data de início é obrigatória quando data final é fornecida", "INVALID_DATE_RANGE", getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10, EndDate: ptr(time.Now())}},
		{"start after end", "a data de início não pode ser maior que a data de fim", "INVALID_DATE_RANGE", getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10, StartDate: ptr(time.Now()), EndDate: ptr(time.Now().AddDate(0, 0, -1))}},
		{"period > 31d", "período máximo de 31 dias", "INVALID_DATE_RANGE", getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10, StartDate: ptr(time.Now().AddDate(0, 0, -40)), EndDate: ptr(time.Now())}},
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

// ---------- 2. Business rule (getActivities) ----------

func TestGetActivities_UserNotInTenant(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT uot."userId"`).
		WithArgs("ghost@x.com", "tenant-123").
		WillReturnError(sql.ErrNoRows)

	_, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "ghost@x.com", Page: 1, Limit: 10}, "tenant-123")
	require.ErrorIs(t, err, errUserNotFoundOrNotInTenant)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetActivities_ResolveUserRepoError(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT uot."userId"`).
		WithArgs("a@b.com", "tenant-123").
		WillReturnError(errors.New("conn refused"))

	_, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	var mcErr *memberclasserrors.MemberClassError
	require.True(t, errors.As(err, &mcErr))
	assert.Equal(t, 500, mcErr.Code)
}

func TestGetActivities_UnionReturnsMixedEvents(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")

	t1 := time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 3, 10, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 3, 10, 11, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "type", "date", "details"}).
		AddRow("evt-3", "certificate", t3, []byte(`{"title":"Curso X","status":"issued"}`)).
		AddRow("evt-2", "lessonCompleted", t2, []byte(`{"lessonId":"l-1","lessonName":"Aula 1","rating":5}`)).
		AddRow("evt-1", "login", t1, nil) // login: SQL returns NULL::jsonb

	expectEventsAndCount(mock, rows, 3)

	resp, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	require.NoError(t, err)
	require.Len(t, resp.Events, 3)

	assert.Equal(t, "certificate", resp.Events[0].Type)
	assert.Equal(t, "lessonCompleted", resp.Events[1].Type)
	assert.Equal(t, "login", resp.Events[2].Type)

	// Login carries no details; the field is omitted from the JSON response.
	assert.Empty(t, resp.Events[2].Details)

	marshaled, err := json.Marshal(resp.Events[2])
	require.NoError(t, err)
	assert.NotContains(t, string(marshaled), `"details"`, "login event must not serialize a details field")

	// Other event types still carry their type-specific payload.
	var certDetails map[string]any
	require.NoError(t, json.Unmarshal(resp.Events[0].Details, &certDetails))
	assert.Equal(t, "Curso X", certDetails["title"])

	assert.Equal(t, int64(3), resp.Pagination.TotalCount)
	assert.Equal(t, "a@b.com", resp.Email)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetActivities_Empty(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	expectEventsAndCount(mock, sqlmock.NewRows([]string{"id", "type", "date", "details"}), 0)

	resp, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	require.NoError(t, err)
	assert.Empty(t, resp.Events)
	assert.Equal(t, int64(0), resp.Pagination.TotalCount)
}

func TestGetActivities_UnionError(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	mock.ExpectQuery(`SELECT id, type, date, details FROM \(`).
		WillReturnError(errors.New("boom"))

	_, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	var mcErr *memberclasserrors.MemberClassError
	require.True(t, errors.As(err, &mcErr))
	assert.Equal(t, 500, mcErr.Code)
}

func TestGetActivities_CountError(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	mock.ExpectQuery(`SELECT id, type, date, details FROM \(`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "date", "details"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
		WillReturnError(errors.New("boom"))

	_, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	var mcErr *memberclasserrors.MemberClassError
	require.True(t, errors.As(err, &mcErr))
	assert.Equal(t, 500, mcErr.Code)
}

func TestGetActivities_Pagination(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")

	rows := sqlmock.NewRows([]string{"id", "type", "date", "details"}).
		AddRow("evt-1", "login", time.Now(), []byte(`{}`))
	expectEventsAndCount(mock, rows, 25)

	resp, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 2, Limit: 10}, "tenant-123")
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Pagination.Page)
	assert.Equal(t, int64(25), resp.Pagination.TotalCount)
	assert.Equal(t, 3, resp.Pagination.TotalPages)
	assert.True(t, resp.Pagination.HasNextPage)
	assert.True(t, resp.Pagination.HasPrevPage)
}

func TestGetActivities_DevMode_NoCacheField(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()
	// default: devMode=true

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	expectEventsAndCount(mock,
		sqlmock.NewRows([]string{"id", "type", "date", "details"}).AddRow("evt-1", "login", time.Now(), nil),
		1,
	)

	resp, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	require.NoError(t, err)
	assert.Nil(t, resp.Cache, "dev mode must not attach cache metadata")
}

func TestGetActivities_ProdMode_CacheMissSetsMetaAndStores(t *testing.T) {
	f, mock, cache, done := newTestFeature(t)
	defer done()
	f.devMode = false

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	expectEventsAndCount(mock,
		sqlmock.NewRows([]string{"id", "type", "date", "details"}).AddRow("evt-1", "login", time.Now(), nil),
		1,
	)

	before := time.Now().UTC()
	resp, err := f.getActivities(context.Background(), getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}, "tenant-123")
	require.NoError(t, err)
	require.NotNil(t, resp.Cache, "prod mode must attach cache metadata")

	assert.WithinDuration(t, before, resp.Cache.CachedAt, 2*time.Second)
	assert.Equal(t, cacheTTL, resp.Cache.RefreshAt.Sub(resp.Cache.CachedAt))
	assert.Len(t, cache.store, 1, "prod mode must write the response to cache")
}

func TestGetActivities_ProdMode_CacheHitReturnsStoredMeta(t *testing.T) {
	f, mock, cache, done := newTestFeature(t)
	defer done()
	f.devMode = false

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")

	storedAt := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	stored := activitiesResponse{
		Email: "a@b.com",
		Events: []event{
			{ID: "cached-1", Type: "login", Date: storedAt},
		},
		Pagination: dto.PaginationMeta{Page: 1, Limit: 10, TotalCount: 1, TotalPages: 1},
		Cache:      &cacheMeta{CachedAt: storedAt, RefreshAt: storedAt.Add(cacheTTL)},
	}
	raw, _ := json.Marshal(stored)
	req := getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}
	startDate, endDate := resolveDateRange(req)
	cache.store[buildCacheKey("tenant-123", "user-1", req, startDate, endDate)] = string(raw)

	resp, err := f.getActivities(context.Background(), req, "tenant-123")
	require.NoError(t, err)
	require.Len(t, resp.Events, 1)
	assert.Equal(t, "cached-1", resp.Events[0].ID)
	require.NotNil(t, resp.Cache)
	assert.Equal(t, storedAt, resp.Cache.CachedAt.UTC())
	assert.NoError(t, mock.ExpectationsWereMet(), "cache hit must not hit the DB event/count queries")
}

func TestBuildCacheKey_IncludesTenantAndUser(t *testing.T) {
	req := getActivitiesRequest{Email: "a@b.com", Page: 1, Limit: 10}
	start, end := resolveDateRange(req)
	keyA := buildCacheKey("tenant-A", "user-1", req, start, end)
	keyB := buildCacheKey("tenant-B", "user-1", req, start, end)
	keyC := buildCacheKey("tenant-A", "user-2", req, start, end)
	assert.NotEqual(t, keyA, keyB, "different tenants must produce different keys")
	assert.NotEqual(t, keyA, keyC, "different users must produce different keys")
}

// ---------- 3. HTTP handler ----------

func TestGetActivities_HTTP_MethodNotAllowed(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()
	w := httptest.NewRecorder()
	f.GetActivities(w, httptest.NewRequest(http.MethodPost, "/api/v1/user/activities", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestGetActivities_HTTP_TenantMissing(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()
	w := httptest.NewRecorder()
	f.GetActivities(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/activities?email=a@b.com", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_API_KEY", body["errorCode"])
}

func TestGetActivities_HTTP_MissingEmail(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()
	w := httptest.NewRecorder()
	f.GetActivities(w, withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activities", nil)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_EMAIL", body["errorCode"])
}

func TestGetActivities_HTTP_UserNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT uot."userId"`).
		WithArgs("ghost@x.com", "tenant-123").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GetActivities(w, withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activities?email=ghost@x.com", nil)))
	assert.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "USER_NOT_FOUND", body["errorCode"])
}

func TestGetActivities_HTTP_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectResolveUserID(mock, "a@b.com", "tenant-123", "user-1")
	rows := sqlmock.NewRows([]string{"id", "type", "date", "details"}).
		AddRow("evt-1", "download", time.Now(), []byte(`{"arquiveId":"a-1","arquiveName":"slides.pdf","size":123456}`))
	expectEventsAndCount(mock, rows, 1)

	w := httptest.NewRecorder()
	f.GetActivities(w, withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/user/activities?email=a@b.com&page=1&limit=10", nil)))

	assert.Equal(t, http.StatusOK, w.Code)
	var body activitiesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "a@b.com", body.Email)
	require.Len(t, body.Events, 1)
	assert.Equal(t, "download", body.Events[0].Type)

	var details map[string]any
	require.NoError(t, json.Unmarshal(body.Events[0].Details, &details))
	assert.Equal(t, "slides.pdf", details["arquiveName"])
}
