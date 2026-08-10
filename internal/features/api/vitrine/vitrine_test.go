package vitrine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

// requestWithTenant builds a GET carrying a tenant and, optionally, chi path
// params — the slice reads ids through chi.URLParam.
func requestWithTenant(target, tenantID string, params map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
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

func vitrineRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "published", "order"})
}

func courseRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
}

func sectionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
}

func moduleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
}

func lessonRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
}

// ---------- 1. Catalog ----------

// The whole tree must come back in four follow-up queries — one per level —
// no matter how many nodes each level holds. sqlmock fails on any extra call.
func TestGetVitrines_LoadsTreeWithOneQueryPerLevel(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).
		WithArgs("t1").
		WillReturnRows(vitrineRows().
			AddRow("v1", "Vitrine 1", true, 1).
			AddRow("v2", "Vitrine 2", true, 2))

	mock.ExpectQuery(`FROM "Course" c`).
		WillReturnRows(courseRows().
			AddRow("c1", "Curso 1", true, 1, "v1").
			AddRow("c2", "Curso 2", true, 2, "v2"))

	mock.ExpectQuery(`FROM "Section" s`).
		WillReturnRows(sectionRows().
			AddRow("s1", "Seção 1", 1, "c1").
			AddRow("s2", "Seção 2", 1, "c2"))

	mock.ExpectQuery(`FROM "Module" m`).
		WillReturnRows(moduleRows().
			AddRow("m1", "Módulo 1", true, 1, "s1").
			AddRow("m2", "Módulo 2", true, 1, "s2"))

	mock.ExpectQuery(`FROM "Lesson" l`).
		WillReturnRows(lessonRows().
			AddRow("l1", "Aula 1", true, "aula-1", "video", "https://cdn/1.mp4", nil, 1, "m1").
			AddRow("l2", "Aula 2", true, nil, nil, nil, nil, 2, "m1"))

	w := httptest.NewRecorder()
	f.GetVitrines(w, requestWithTenant("/", "t1", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var resp vitrineResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Vitrines, 2)

	// Children land under the right parent.
	require.Len(t, resp.Vitrines[0].Courses, 1)
	assert.Equal(t, "c1", resp.Vitrines[0].Courses[0].ID)
	require.Len(t, resp.Vitrines[1].Courses, 1)
	assert.Equal(t, "c2", resp.Vitrines[1].Courses[0].ID)

	lessons := resp.Vitrines[0].Courses[0].Sections[0].Modules[0].Lessons
	require.Len(t, lessons, 2)
	assert.Equal(t, "l1", lessons[0].ID)
	require.NotNil(t, lessons[0].Slug)
	assert.Equal(t, "aula-1", *lessons[0].Slug)
	// NULL columns stay nil so `omitempty` drops them.
	assert.Nil(t, lessons[1].Slug)
	assert.Nil(t, lessons[1].MediaURL)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// An empty catalog must not run the child queries at all.
func TestGetVitrines_EmptyCatalogSkipsChildQueries(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).WithArgs("t1").WillReturnRows(vitrineRows())

	w := httptest.NewRecorder()
	f.GetVitrines(w, requestWithTenant("/", "t1", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var resp vitrineResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Vitrines)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A node with no children must omit the key rather than send an empty array —
// clients distinguish the two.
func TestGetVitrines_OmitsEmptyChildKeys(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).
		WillReturnRows(vitrineRows().AddRow("v1", "Vitrine 1", true, nil))
	mock.ExpectQuery(`FROM "Course" c`).WillReturnRows(courseRows())

	w := httptest.NewRecorder()
	f.GetVitrines(w, requestWithTenant("/", "t1", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"courses"`)
	assert.NotContains(t, w.Body.String(), `"order"`)
}

func TestGetVitrines_RequiresTenant(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetVitrines(w, requestWithTenant("/", "", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_API_KEY", errorCodeOf(t, w))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVitrines_RejectsNonGet(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetVitrines(w, httptest.NewRequest(http.MethodPost, "/", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ---------- 2. Detail lookups ----------

func TestDetailLookups_NotFound(t *testing.T) {
	cases := []struct {
		name       string
		queryRegex string
		param      string
		call       func(f *Feature, w http.ResponseWriter, r *http.Request)
		wantMsg    string
	}{
		{"vitrine", `FROM "Vitrine" v`, "vitrineId", (*Feature).GetVitrine, "Vitrine não encontrada"},
		{"course", `FROM "Course" c`, "courseId", (*Feature).GetCourse, "Curso não encontrado"},
		{"module", `FROM "Module" m`, "moduleId", (*Feature).GetModule, "Módulo não encontrado"},
		{"lesson", `FROM "Lesson" l`, "lessonId", (*Feature).GetLesson, "Aula não encontrada"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			mock.ExpectQuery(tc.queryRegex).
				WithArgs("missing", "t1").
				WillReturnError(sql.ErrNoRows)

			w := httptest.NewRecorder()
			tc.call(f, w, requestWithTenant("/x", "t1", map[string]string{tc.param: "missing"}))

			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "NOT_FOUND", errorCodeOf(t, w))
			assert.Contains(t, w.Body.String(), tc.wantMsg)
		})
	}
}

// Every detail query must carry the tenant id: without it, one tenant could
// read another's catalog by guessing an id.
func TestDetailLookups_AreTenantScoped(t *testing.T) {
	cases := []struct {
		name       string
		queryRegex string
		param      string
		call       func(f *Feature, w http.ResponseWriter, r *http.Request)
		rows       func() *sqlmock.Rows
	}{
		{
			"vitrine", `WHERE v\.id = \$1 AND v\."tenantId" = \$2`, "vitrineId", (*Feature).GetVitrine,
			func() *sqlmock.Rows { return vitrineRows().AddRow("v1", "V", true, nil) },
		},
		{
			"course", `WHERE c\.id = \$1 AND v\."tenantId" = \$2`, "courseId", (*Feature).GetCourse,
			func() *sqlmock.Rows { return vitrineRows().AddRow("c1", "C", true, nil) },
		},
		{
			"module", `WHERE m\.id = \$1 AND v\."tenantId" = \$2`, "moduleId", (*Feature).GetModule,
			func() *sqlmock.Rows { return vitrineRows().AddRow("m1", "M", true, nil) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			mock.ExpectQuery(tc.queryRegex).
				WithArgs("id-1", "t1").
				WillReturnRows(tc.rows())

			w := httptest.NewRecorder()
			tc.call(f, w, requestWithTenant("/x", "t1", map[string]string{tc.param: "id-1"}))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDetailLookups_RequireTenant(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetVitrine(w, requestWithTenant("/v1", "", map[string]string{"vitrineId": "v1"}))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Without ?includeChildren the subtree queries must not run.
func TestGetVitrine_WithoutIncludeChildrenSkipsSubtree(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).
		WithArgs("v1", "t1").
		WillReturnRows(vitrineRows().AddRow("v1", "Vitrine 1", true, 3))

	w := httptest.NewRecorder()
	f.GetVitrine(w, requestWithTenant("/v1", "t1", map[string]string{"vitrineId": "v1"}))

	require.Equal(t, http.StatusOK, w.Code)

	var resp vitrineDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v1", resp.Vitrine.ID)
	assert.Empty(t, resp.Vitrine.Courses)
	require.NotNil(t, resp.Vitrine.Order)
	assert.Equal(t, 3, *resp.Vitrine.Order)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVitrine_WithIncludeChildrenLoadsSubtree(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).
		WithArgs("v1", "t1").
		WillReturnRows(vitrineRows().AddRow("v1", "Vitrine 1", true, nil))
	mock.ExpectQuery(`FROM "Course" c`).
		WillReturnRows(courseRows().AddRow("c1", "Curso 1", true, nil, "v1"))
	mock.ExpectQuery(`FROM "Section" s`).
		WillReturnRows(sectionRows().AddRow("s1", "Seção 1", nil, "c1"))
	mock.ExpectQuery(`FROM "Module" m`).
		WillReturnRows(moduleRows().AddRow("m1", "Módulo 1", true, nil, "s1"))
	mock.ExpectQuery(`FROM "Lesson" l`).
		WillReturnRows(lessonRows().AddRow("l1", "Aula 1", true, nil, nil, nil, nil, nil, "m1"))

	w := httptest.NewRecorder()
	f.GetVitrine(w, requestWithTenant("/v1?includeChildren=true", "t1", map[string]string{"vitrineId": "v1"}))

	require.Equal(t, http.StatusOK, w.Code)

	var resp vitrineDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Vitrine.Courses, 1)
	require.Len(t, resp.Vitrine.Courses[0].Sections, 1)
	require.Len(t, resp.Vitrine.Courses[0].Sections[0].Modules, 1)
	require.Len(t, resp.Vitrine.Courses[0].Sections[0].Modules[0].Lessons, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetModule_IncludeChildrenLoadsOnlyLessons(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Module" m`).
		WithArgs("m1", "t1").
		WillReturnRows(vitrineRows().AddRow("m1", "Módulo 1", true, nil))
	mock.ExpectQuery(`FROM "Lesson" l`).
		WillReturnRows(lessonRows().AddRow("l1", "Aula 1", true, nil, nil, nil, nil, nil, "m1"))

	w := httptest.NewRecorder()
	f.GetModule(w, requestWithTenant("/m1?includeChildren=1", "t1", map[string]string{"moduleId": "m1"}))

	require.Equal(t, http.StatusOK, w.Code)

	var resp moduleDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Module.Lessons, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLesson_MapsNullableColumns(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Lesson" l`).
		WithArgs("l1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
			AddRow("l1", "Aula 1", true, "aula-1", "video", "https://cdn/1.mp4", "https://cdn/t.jpg", 7))

	w := httptest.NewRecorder()
	f.GetLesson(w, requestWithTenant("/l1", "t1", map[string]string{"lessonId": "l1"}))

	require.Equal(t, http.StatusOK, w.Code)

	var resp lessonDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Lesson.Slug)
	assert.Equal(t, "aula-1", *resp.Lesson.Slug)
	require.NotNil(t, resp.Lesson.Thumbnail)
	require.NotNil(t, resp.Lesson.Order)
	assert.Equal(t, 7, *resp.Lesson.Order)
}

// ---------- 3. Parsing and error mapping ----------

func TestParseIncludeChildren(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"true":  true,
		"1":     true,
		"false": false,
		"0":     false,
		"yes":   false, // not a Go boolean literal, so it reads as false
	}

	for raw, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/?includeChildren="+raw, nil)
		assert.Equal(t, want, parseIncludeChildren(r), "includeChildren=%q", raw)
	}
}

// A driver failure must surface as a 500 whose body carries the Portuguese
// message, never the driver text.
func TestQueryFailureIsFiveHundred(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Vitrine" v`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GetVitrines(w, requestWithTenant("/", "t1", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "erro ao buscar")
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// 405 and unmapped codes use {error, message}; mapped codes use
// {ok, error, errorCode}. Both shapes are part of the contract.
func TestErrorBodyShapes(t *testing.T) {
	t.Run("unmapped code uses error/message", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Method Not Allowed", body["error"])
		assert.Equal(t, "Method not allowed", body["message"])
		assert.NotContains(t, body, "errorCode")
	})

	t.Run("mapped code uses ok/error/errorCode", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeCustomError(w, http.StatusNotFound, "Aula não encontrada", "NOT_FOUND")

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, false, body["ok"])
		assert.Equal(t, "Aula não encontrada", body["error"])
		assert.Equal(t, "NOT_FOUND", body["errorCode"])
	})
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
