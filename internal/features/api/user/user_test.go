package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeCache struct {
	mu          sync.Mutex
	store       map[string]string
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

func requestWithTenant(target, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if tenantID == "" {
		return req
	}
	ctx := context.WithValue(req.Context(), tenant.ContextKey, &tenant.Tenant{ID: tenantID})
	return req.WithContext(ctx)
}

// expectMemberLookup queues the email → user id resolution the purchases and
// completed-lessons endpoints run first.
func expectMemberLookup(mock sqlmock.Sqlmock, email, tenantID, userID string) {
	mock.ExpectQuery(`FROM "User" u`).
		WithArgs(email, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
}

func errorCodeOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		ErrorCode string `json:"errorCode"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.ErrorCode
}

// ---------- 1. Shared member lookup ----------

// A user who exists but belongs to another tenant must be indistinguishable
// from one who does not exist: the endpoint must not confirm the account.
func TestMemberID_UnknownOrOtherTenantIsNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" u`).
		WithArgs("ghost@example.com", "t1").
		WillReturnError(sql.ErrNoRows)

	_, err := f.memberID(context.Background(), "ghost@example.com", "t1")
	assert.ErrorIs(t, err, errUserNotInTenant)
}

// A driver failure is a 500, not a 404 — the previous implementation reported
// any lookup error as "user not found".
func TestMemberID_DatabaseFailureIsNotNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" u`).WillReturnError(errors.New("connection reset"))

	_, err := f.memberID(context.Background(), "a@example.com", "t1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUserNotInTenant)
}

// The lookup must be scoped by tenant in SQL, not filtered afterwards.
func TestMemberID_QueryCarriesTenant(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`uot\."tenantId" = \$2`).
		WithArgs("a@example.com", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))

	userID, err := f.memberID(context.Background(), "a@example.com", "t1")
	require.NoError(t, err)
	assert.Equal(t, "u1", userID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 2. Pagination parsing ----------

func TestParsePageAndLimit(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		page, err := parsePage(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, page)

		limit, err := parseLimit(nil)
		require.NoError(t, err)
		assert.Equal(t, 10, limit)
	})

	t.Run("rejects out-of-range values", func(t *testing.T) {
		_, err := parsePage(map[string][]string{"page": {"0"}})
		assert.EqualError(t, err, "page must be a positive integer")

		_, err = parsePage(map[string][]string{"page": {"abc"}})
		assert.EqualError(t, err, "page must be a positive integer")

		_, err = parseLimit(map[string][]string{"limit": {"101"}})
		assert.EqualError(t, err, "limit must be between 1 and 100")

		_, err = parseLimit(map[string][]string{"limit": {"0"}})
		assert.EqualError(t, err, "limit must be between 1 and 100")
	})
}

// ---------- 3. GET /user/informations ----------

func TestGetUserInformations_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	lastAccess := time.Date(2026, 5, 1, 8, 30, 0, 0, time.UTC)
	// Pinning "LoginEvent" here is the point of the expectation. lastAccess used
	// to read "SystemLog", which stopped receiving logins and returned null for
	// every member — a swap sqlmock cannot notice unless the table is matched.
	mock.ExpectQuery(`FROM "LoginEvent" le`).
		WithArgs("t1", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}).
			AddRow("u1", "a@example.com", "Aluno", true, lastAccess))

	assignedAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC)
	lastEventAt := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`FROM "MemberOnDelivery"`).
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"memberId", "deliveryId", "assignedAt", "delivery_name",
			"status", "expiresAt", "platform",
			"externalSubscriptionId", "canceledAt", "lastEventAt",
		}).AddRow("u1", "d1", assignedAt, "Curso Base",
			"active", expiresAt, "hotmart", "sub_123", nil, lastEventAt))

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp userInformationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Users, 1)
	assert.Equal(t, "u1", resp.Users[0].UserID)
	assert.True(t, resp.Users[0].IsPaid)
	require.NotNil(t, resp.Users[0].LastAccess)
	// The layout is not RFC3339: clients parse the fixed .000Z suffix.
	assert.Equal(t, "2026-05-01T08:30:00.000Z", *resp.Users[0].LastAccess)
	require.Len(t, resp.Users[0].Deliveries, 1)
	d := resp.Users[0].Deliveries[0]
	assert.Equal(t, "Curso Base", d.Name)
	assert.Equal(t, "active", d.Status)
	require.NotNil(t, d.ExpiresAt)
	assert.Equal(t, "2027-04-01T00:00:00.000Z", *d.ExpiresAt)
	require.NotNil(t, d.Platform)
	assert.Equal(t, "hotmart", *d.Platform)
	require.NotNil(t, d.ExternalSubscriptionID)
	assert.Equal(t, "sub_123", *d.ExternalSubscriptionID)
	require.NotNil(t, d.LastEventAt)
	// Not cancelled: the field stays null rather than becoming a zero time.
	assert.Nil(t, d.CanceledAt)
	assert.Equal(t, int64(1), resp.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A lifetime purchase has no expiry and a hand-made grant has no gateway
// origin. Both must stay null in the response: flattening them to "" would make
// a manual grant indistinguishable from one that lost its platform.
func TestGetUserInformations_LifetimeGrantKeepsNulls(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery(`FROM "LoginEvent" le`).
		WithArgs("t1", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}).
			AddRow("u1", "a@example.com", "Aluno", false, nil))

	assignedAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`FROM "MemberOnDelivery"`).
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"memberId", "deliveryId", "assignedAt", "delivery_name",
			"status", "expiresAt", "platform",
			"externalSubscriptionId", "canceledAt", "lastEventAt",
		}).AddRow("u1", "d1", assignedAt, "Curso Vitalício",
			"active", nil, nil, nil, nil, nil))

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp userInformationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Users, 1)
	assert.Nil(t, resp.Users[0].LastAccess)
	require.Len(t, resp.Users[0].Deliveries, 1)

	d := resp.Users[0].Deliveries[0]
	assert.Equal(t, "active", d.Status)
	assert.Nil(t, d.ExpiresAt)
	assert.Nil(t, d.Platform)
	assert.Nil(t, d.ExternalSubscriptionID)
	assert.Nil(t, d.CanceledAt)
	assert.Nil(t, d.LastEventAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// With ?email the endpoint first checks membership, so a stranger's address
// yields 404 rather than an empty page.
func TestGetUserInformations_EmailOutsideTenantIsNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" u`).
		WithArgs("ghost@example.com", "t1").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations?email=ghost@example.com", "t1"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "USER_NOT_FOUND", errorCodeOf(t, w))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The email filter is bound as $4, so its placeholder must not collide with
// the pagination arguments.
func TestGetUserInformations_EmailFilterBindsAsFourthArg(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("t1", "a@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`AND u\.email = \$4`).
		WithArgs("t1", 10, 0, "a@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}))

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An empty page must not run the deliveries query.
func TestGetUserInformations_EmptyPageSkipsDeliveries(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`WITH users_base AS`).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "email", "name", "is_paid", "last_access"}))

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations", "t1"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserInformations_RequiresTenant(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetUserInformations(w, requestWithTenant("/informations", ""))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetUserInformations_ServesFromCache(t *testing.T) {
	f, mock, c, done := newTestFeature(t)
	defer done()

	cached, err := json.Marshal(userInformationsResponse{
		Users: []userInformation{{UserID: "cached"}},
	})
	require.NoError(t, err)
	c.getOverride = func(string) (string, error) { return string(cached), nil }

	resp, err := f.getInformations(context.Background(), "t1", "", 1, 10)
	require.NoError(t, err)
	require.Len(t, resp.Users, 1)
	assert.Equal(t, "cached", resp.Users[0].UserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 4. GET /users/purchases ----------

func TestGetUserPurchases_RequiresEmail(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetUserPurchases(w, requestWithTenant("/purchases", "t1"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "MISSING_EMAIL", errorCodeOf(t, w))
}

func TestGetUserPurchases_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")
	mock.ExpectQuery(`WITH filtered AS`).
		WithArgs("u1", "t1", sqlmock.AnyArg(), 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}).
			AddRow("e1", "purchase", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z", int64(3)))

	w := httptest.NewRecorder()
	f.GetUserPurchases(w, requestWithTenant("/purchases?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp userPurchasesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Purchases, 1)
	assert.Equal(t, "purchase", resp.Purchases[0].Type)
	assert.Equal(t, int64(3), resp.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An empty page has no row carrying the total, so the count stays zero rather
// than reporting a stale value.
func TestGetUserPurchases_EmptyPageReportsZeroTotal(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`WITH filtered AS`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}))

	purchases, total, err := f.queryPurchases(context.Background(), "u1", "t1", []string{"purchase"}, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, purchases)
	assert.Equal(t, int64(0), total)
}

func TestGetUserPurchases_UnknownEmailIsNotFound(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "User" u`).WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GetUserPurchases(w, requestWithTenant("/purchases?email=ghost@example.com", "t1"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "USER_NOT_FOUND", errorCodeOf(t, w))
}

// ---------- 5. GET /user/lessons/completed ----------

// The date filters accept a bare calendar date, not only full RFC3339. A caller
// filtering by day sends 2026-08-10, and rejecting that with "formato de data
// inválido" while the message named no accepted format is how this endpoint
// first looked broken.
func TestParseLessonsCompleted_AcceptsBareDates(t *testing.T) {
	req, err := parseLessonsCompleted(map[string][]string{
		"email":     {"a@example.com"},
		"startDate": {"2026-08-10"},
		"endDate":   {"2026-08-13"},
	})
	require.NoError(t, err)
	require.NotNil(t, req.StartDate)
	require.NotNil(t, req.EndDate)

	assert.Equal(t, "2026-08-10T00:00:00Z", req.StartDate.UTC().Format(time.RFC3339))

	// The end bound closes at the end of the named day. Resolved to midnight it
	// would drop almost all of the 13th, and the caller asked for the 13th.
	lateOnTheLastDay := time.Date(2026, 8, 13, 22, 15, 0, 0, time.UTC)
	assert.False(t, lateOnTheLastDay.After(*req.EndDate),
		"endDate %s excludes %s", req.EndDate, lateOnTheLastDay)
}

// RFC3339 keeps meaning exactly what it says: a caller who spells out a time is
// not rounded to a day boundary.
func TestParseLessonsCompleted_RFC3339IsNotWidened(t *testing.T) {
	req, err := parseLessonsCompleted(map[string][]string{
		"email":     {"a@example.com"},
		"startDate": {"2026-08-10T14:32:05Z"},
		"endDate":   {"2026-08-13T09:00:00Z"},
	})
	require.NoError(t, err)
	assert.Equal(t, "2026-08-10T14:32:05Z", req.StartDate.UTC().Format(time.RFC3339))
	assert.Equal(t, "2026-08-13T09:00:00Z", req.EndDate.UTC().Format(time.RFC3339))
}

func TestGetLessonsCompleted_ValidationCodes(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantCode string
	}{
		{"missing email", "", "MISSING_EMAIL"},
		{"page not a number", "?email=a@example.com&page=abc", "INVALID_PAGINATION"},
		{"page below one", "?email=a@example.com&page=0", "INVALID_PAGINATION"},
		{"bad start date", "?email=a@example.com&startDate=nope", "INVALID_DATE_FORMAT"},
		{"end without start", "?email=a@example.com&endDate=2026-01-01T00:00:00Z", "INVALID_DATE_RANGE"},
		{"inverted range", "?email=a@example.com&startDate=2026-02-01T00:00:00Z&endDate=2026-01-01T00:00:00Z", "INVALID_DATE_RANGE"},
		{"window over 31 days", "?email=a@example.com&startDate=2026-01-01T00:00:00Z&endDate=2026-03-01T00:00:00Z", "INVALID_DATE_RANGE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.GetLessonsCompleted(w, requestWithTenant("/lessons/completed"+tc.query, "t1"))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.wantCode, errorCodeOf(t, w))
		})
	}
}

func TestGetLessonsCompleted_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")

	completedAt := time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC)
	// The Vitrine join is pinned in both expectations. Scoping through
	// CourseOnDelivery instead asks whether the course is bundled into an offer
	// rather than whether it belongs to the tenant, and returned an empty page
	// for members who had completed lessons.
	mock.ExpectQuery(`JOIN "Vitrine" v ON v\.id = c\."vitrineId"`).
		WillReturnRows(sqlmock.NewRows([]string{"completed_at", "lesson_name", "course_name"}).
			AddRow(completedAt, "Aula 1", "Curso 1"))
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT.*JOIN "Vitrine" v`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	w := httptest.NewRecorder()
	f.GetLessonsCompleted(w, requestWithTenant("/lessons/completed?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp lessonsCompletedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	require.Len(t, resp.Data.CompletedLessons, 1)
	assert.Equal(t, "Aula 1", resp.Data.CompletedLessons[0].LessonName)
	assert.Equal(t, "2026-06-01T14:00:00.000Z", resp.Data.CompletedLessons[0].CompletedAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Tenant scope belongs to the ownership chain, not the entitlement one.
// Delivery describes what a member bought; Vitrine describes what the tenant
// owns. Asking the first question returned nothing for lessons whose course is
// not bound to a Delivery through CourseOnDelivery — including every course
// granted at vitrine, module or lesson level, since deliveries bind at any of
// those. /api/v1/user/activities always used the ownership chain, which is how
// the two endpoints came to disagree about the same Read row.
func TestCompletedLessonsSQL_ScopesByOwnershipNotEntitlement(t *testing.T) {
	for name, query := range map[string]string{
		"page":  sqlCompletedLessons,
		"count": sqlCountCompletedLessons,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, query, `JOIN "Vitrine" v`,
				"tenant scope must walk course -> vitrine")
			assert.Contains(t, query, `v."tenantId"`,
				"the tenant filter belongs on Vitrine")
			assert.NotContains(t, query, "CourseOnDelivery",
				"entitlement tables hide lessons the member has genuinely completed")
			assert.NotContains(t, query, `JOIN "Delivery"`,
				"entitlement tables hide lessons the member has genuinely completed")
		})
	}
}

// The optional course filter takes $5, pushing LIMIT/OFFSET to $6/$7.
func TestQueryCompletedLessons_CourseFilterShiftsPlaceholders(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`LIMIT \$6 OFFSET \$7`).
		WithArgs("u1", start, end, "t1", "c1", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"completed_at", "lesson_name", "course_name"}))
	mock.ExpectQuery(`AND c\.id = \$5`).
		WithArgs("u1", start, end, "t1", "c1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	_, _, err := f.queryCompletedLessons(context.Background(), "u1", "t1", start, end, "c1", 1, 10)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveWindow(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("no dates defaults to the last 31 whole days", func(t *testing.T) {
		start, end := resolveWindow(lessonsCompletedRequest{}, now)
		// 2026-05-16 through 2026-06-15 inclusive is 31 calendar days.
		assert.Equal(t, time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC), start)
		assert.Equal(t, time.Date(2026, 6, 15, 23, 59, 59, 999999999, time.UTC), end)
	})

	t.Run("the default window covers the whole first day", func(t *testing.T) {
		// The old offset-from-now default cut at 12:00 on the oldest day and
		// hid anything completed before it.
		start, _ := resolveWindow(lessonsCompletedRequest{}, now)
		assert.Equal(t, 0, start.Hour(), "oldest day must start at midnight")
	})

	t.Run("the default window fits inside the caller-facing maximum", func(t *testing.T) {
		start, end := resolveWindow(lessonsCompletedRequest{}, now)
		assert.LessOrEqual(t, end.Sub(start), maxCompletedWindow,
			"default must not be wider than a caller is allowed to request")
	})

	t.Run("the default resolves in UTC whatever zone now carries", func(t *testing.T) {
		// datefilter.Parse resolves an explicit date-only bound in UTC. A
		// default read in the process zone would measure a different day.
		saoPaulo := time.FixedZone("BRT", -3*60*60)
		start, end := resolveWindow(lessonsCompletedRequest{}, now.In(saoPaulo))
		assert.Equal(t, time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC), start)
		assert.Equal(t, time.Date(2026, 6, 15, 23, 59, 59, 999999999, time.UTC), end)
	})

	t.Run("start only keeps the spelled-out time and closes that day", func(t *testing.T) {
		// datefilter.Parse takes RFC3339 literally; rounding the start down
		// here would override the time the caller meant.
		day := time.Date(2026, 3, 10, 17, 45, 0, 0, time.UTC)
		start, end := resolveWindow(lessonsCompletedRequest{StartDate: &day}, now)
		assert.Equal(t, day, start)
		assert.Equal(t, time.Date(2026, 3, 10, 23, 59, 59, 999999999, time.UTC), end)
	})

	t.Run("start only never spills into the next day", func(t *testing.T) {
		day := time.Date(2026, 3, 10, 23, 59, 0, 0, time.UTC)
		_, end := resolveWindow(lessonsCompletedRequest{StartDate: &day}, now)
		assert.Equal(t, 10, end.Day())
	})

	t.Run("a date-only startDate still covers its whole day", func(t *testing.T) {
		// The date-only form arrives already at midnight from datefilter.Parse,
		// so dropping the widening here must not narrow it.
		req, err := parseLessonsCompleted(url.Values{
			"email":     []string{"a@example.com"},
			"startDate": []string{"2026-03-10"},
		})
		require.NoError(t, err)
		start, end := resolveWindow(*req, now)
		assert.Equal(t, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), start)
		assert.Equal(t, time.Date(2026, 3, 10, 23, 59, 59, 999999999, time.UTC), end)
	})

	t.Run("both dates are used as given", func(t *testing.T) {
		a := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
		b := time.Date(2026, 3, 20, 6, 0, 0, 0, time.UTC)
		start, end := resolveWindow(lessonsCompletedRequest{StartDate: &a, EndDate: &b}, now)
		assert.Equal(t, a, start)
		assert.Equal(t, b, end)
	})
}

func TestGetLessonsCompleted_RequiresTenant(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetLessonsCompleted(w, requestWithTenant("/lessons/completed?email=a@example.com", ""))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_API_KEY", errorCodeOf(t, w))
}

// ---------- 6. Method guards ----------

func TestHandlers_RejectNonGet(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	handlers := map[string]http.HandlerFunc{
		"informations":      f.GetUserInformations,
		"purchases":         f.GetUserPurchases,
		"lessons/completed": f.GetLessonsCompleted,
	}

	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(http.MethodPost, "/"+name, nil))
			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}
