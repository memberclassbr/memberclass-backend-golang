package comment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return New(db, fakeLogger{}), mock, func() { _ = db.Close() }
}

func request(method, target, tenantID, body string, params map[string]string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}

	ctx := req.Context()
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	if tenantID != "" {
		ctx = context.WithValue(ctx, constants.TenantContextKey, &tenant.Tenant{ID: tenantID})
	}
	return req.WithContext(ctx)
}

func commentRowColumns() []string {
	return []string{
		"id", "createdAt", "updatedAt", "published", "question", "answer",
		"lesson_name", "course_name", "user_name", "user_email",
	}
}

func bodyOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// ---------- 1. Request parsing ----------

func TestParseGetComments(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		req, err := parseGetComments(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, req.Page)
		assert.Equal(t, 10, req.Limit)
	})

	t.Run("non-numeric pagination is an error", func(t *testing.T) {
		_, err := parseGetComments(map[string][]string{"page": {"x"}})
		assert.Error(t, err)

		_, err = parseGetComments(map[string][]string{"limit": {"x"}})
		assert.Error(t, err)
	})
}

// Unknown filter values widen the result set instead of failing the request —
// they come from UI dropdowns.
func TestValidate_DropsUnknownFilterValues(t *testing.T) {
	req := getCommentsRequest{Page: 1, Limit: 10, Status: "banana", Answered: "maybe"}
	require.NoError(t, req.validate())
	assert.Empty(t, req.Status)
	assert.Empty(t, req.Answered)
}

func TestValidate_NormalisesCase(t *testing.T) {
	req := getCommentsRequest{Page: 1, Limit: 10, Status: "APPROVED", Answered: "TRUE"}
	require.NoError(t, req.validate())
	assert.Equal(t, "approved", req.Status)
	assert.Equal(t, "true", req.Answered)
}

func TestValidate_RejectsPaginationOutOfRange(t *testing.T) {
	for _, req := range []getCommentsRequest{
		{Page: 0, Limit: 10},
		{Page: 1, Limit: 0},
		{Page: 1, Limit: 101},
	} {
		assert.Error(t, req.validate())
	}
}

// ---------- 2. Filters ----------

// The page query and the count query must build identical WHERE clauses, or
// the reported total will not match the rows returned.
func TestCommentFilters_PageAndCountAgree(t *testing.T) {
	req := getCommentsRequest{
		Page: 1, Limit: 10,
		Email: "a@example.com", Status: "approved", CourseID: "c1", Answered: "true",
	}

	pageClause, pageArgs := commentFilters(req, []any{"t1"})
	countClause, countArgs := commentFilters(req, []any{"t1"})

	assert.Equal(t, pageClause, countClause)
	assert.Equal(t, pageArgs, countArgs)
}

func TestCommentFilters_Placeholders(t *testing.T) {
	t.Run("pendent uses IS NULL and binds nothing", func(t *testing.T) {
		clause, args := commentFilters(getCommentsRequest{Status: "pendent"}, []any{"t1"})
		assert.Contains(t, clause, "c.published IS NULL")
		assert.Len(t, args, 1)
	})

	t.Run("rejected binds false", func(t *testing.T) {
		clause, args := commentFilters(getCommentsRequest{Status: "rejected"}, []any{"t1"})
		assert.Contains(t, clause, "c.published = $2")
		require.Len(t, args, 2)
		assert.Equal(t, false, args[1])
	})

	t.Run("email is a substring match", func(t *testing.T) {
		_, args := commentFilters(getCommentsRequest{Email: "ana"}, []any{"t1"})
		require.Len(t, args, 2)
		assert.Equal(t, "%ana%", args[1])
	})

	t.Run("answered=false covers null and empty", func(t *testing.T) {
		clause, _ := commentFilters(getCommentsRequest{Answered: "false"}, []any{"t1"})
		assert.Contains(t, clause, "c.answer IS NULL OR c.answer = ''")
	})

	t.Run("placeholders stay in order when several filters combine", func(t *testing.T) {
		clause, args := commentFilters(getCommentsRequest{
			Email: "ana", Status: "approved", CourseID: "c1",
		}, []any{"t1"})
		assert.Contains(t, clause, "u.email ILIKE $2")
		assert.Contains(t, clause, "c.published = $3")
		assert.Contains(t, clause, "course.id = $4")
		assert.Len(t, args, 4)
	})
}

// ---------- 3. GET /comments ----------

// The listing is mounted behind two different middlewares. Neither may reach
// the query without a tenant: the previous handler only guarded the /api/v1
// path and dereferenced a nil tenant on the other one.
func TestGetComments_RequiresTenantOnEveryPath(t *testing.T) {
	for _, path := range []string{"/api/v1/comments", "/api/comments"} {
		t.Run(path, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.GetComments(w, request(http.MethodGet, path, "", "", nil))

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, "INVALID_API_KEY", bodyOf(t, w)["errorCode"])
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGetComments_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	createdAt := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`FROM "Comment" c`).
		WithArgs("t1", 10, 0).
		WillReturnRows(sqlmock.NewRows(commentRowColumns()).
			AddRow("cm1", createdAt, createdAt, true, "Pergunta?", "Resposta", "Aula 1", "Curso 1", "Ana", "ana@example.com"))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodGet, "/api/v1/comments", "t1", "", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var resp commentsPaginationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Comments, 1)
	assert.Equal(t, "cm1", resp.Comments[0].ID)
	require.NotNil(t, resp.Comments[0].Answer)
	assert.Equal(t, "Resposta", *resp.Comments[0].Answer)
	require.NotNil(t, resp.Comments[0].Published)
	assert.True(t, *resp.Comments[0].Published)
	assert.Equal(t, int64(1), resp.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An empty answer column means "unanswered", so it must not surface as "".
func TestGetComments_EmptyAnswerIsNull(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	now := time.Now()
	mock.ExpectQuery(`FROM "Comment" c`).
		WillReturnRows(sqlmock.NewRows(commentRowColumns()).
			AddRow("cm1", now, now, nil, "Pergunta?", "", "Aula", "Curso", "", "a@example.com"))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodGet, "/api/v1/comments", "t1", "", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var resp commentsPaginationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Comments, 1)
	assert.Nil(t, resp.Comments[0].Answer)
	assert.Nil(t, resp.Comments[0].Published)
}

// An empty page must serialise as [], not null.
func TestGetComments_EmptyListIsArray(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Comment" c`).WillReturnRows(sqlmock.NewRows(commentRowColumns()))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodGet, "/api/v1/comments", "t1", "", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"comments":[]`)
}

func TestGetComments_BadPaginationCarriesCode(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodGet, "/api/v1/comments?page=0", "t1", "", nil))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_PAGINATION", bodyOf(t, w)["errorCode"])
}

func TestGetComments_QueryFailureIsFiveHundred(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Comment" c`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodGet, "/api/v1/comments", "t1", "", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// ---------- 4. PATCH /comments/{commentID} ----------

func TestUpdateComment_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT c\.id`).
		WithArgs("cm1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("cm1"))
	mock.ExpectExec(`UPDATE "Comment"`).
		WithArgs("cm1", "Minha resposta", true, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	now := time.Now()
	mock.ExpectQuery(`AND c\.id = \$2`).
		WithArgs("t1", "cm1").
		WillReturnRows(sqlmock.NewRows(commentRowColumns()).
			AddRow("cm1", now, now, true, "Pergunta?", "Minha resposta", "Aula", "Curso", "Ana", "ana@example.com"))

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "t1",
		`{"answer":"Minha resposta","published":true}`, map[string]string{"commentID": "cm1"}))

	require.Equal(t, http.StatusOK, w.Code)

	body := bodyOf(t, w)
	assert.Equal(t, true, body["ok"])
	assert.NotNil(t, body["comment"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The UPDATE is keyed only by comment id, so the tenant check has to happen
// first — without it one tenant could answer another's comments.
func TestUpdateComment_OtherTenantCannotWrite(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT c\.id`).
		WithArgs("cm1", "t2").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "t2",
		`{"answer":"x"}`, map[string]string{"commentID": "cm1"}))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "COMMENT_NOT_FOUND", bodyOf(t, w)["errorCode"])
	// No ExpectExec was queued: reaching the UPDATE would fail the test.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateComment_AnswerIsRequired(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "t1",
		`{"answer":""}`, map[string]string{"commentID": "cm1"}))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An absent `published` means not published, and must not be confused with an
// explicit false.
func TestUpdateComment_AbsentPublishedDefaultsToFalse(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT c\.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("cm1"))
	mock.ExpectExec(`UPDATE "Comment"`).
		WithArgs("cm1", "resposta", false, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	now := time.Now()
	mock.ExpectQuery(`AND c\.id = \$2`).
		WillReturnRows(sqlmock.NewRows(commentRowColumns()).
			AddRow("cm1", now, now, false, "q", "resposta", "Aula", "Curso", "", "a@example.com"))

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "t1",
		`{"answer":"resposta"}`, map[string]string{"commentID": "cm1"}))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateComment_MalformedBody(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "t1",
		`{not json`, map[string]string{"commentID": "cm1"}))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
}

func TestUpdateComment_RequiresTenant(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodPatch, "/api/v1/comments/cm1", "",
		`{"answer":"x"}`, map[string]string{"commentID": "cm1"}))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- 5. Method guards ----------

func TestHandlers_RejectWrongMethod(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetComments(w, request(http.MethodPost, "/api/v1/comments", "t1", "", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	f.UpdateComment(w, request(http.MethodGet, "/api/v1/comments/cm1", "t1", "", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
