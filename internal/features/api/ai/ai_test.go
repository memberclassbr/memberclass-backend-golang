package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInternalKey = "internal-key"

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

	cfg := &config.Config{Auth: config.Auth{InternalAPIKey: testInternalKey}}
	return New(db, cfg, fakeLogger{}), mock, func() { _ = db.Close() }
}

// internalRequest builds a GET carrying the internal API key.
func internalRequest(target string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("x-internal-api-key", testInternalKey)
	return req
}

func bodyOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func lessonColumns() []string {
	return []string{
		"id", "name", "slug", "type", "mediaUrl", "thumbnail", "content",
		"transcriptionCompleted", "module_id", "module_name", "section_id",
		"section_name", "course_id", "course_name", "vitrine_id", "vitrine_name",
	}
}

// ---------- 1. Authorisation ----------

// Both endpoints are internal-only. A missing or wrong key must be rejected
// before any query runs, and a request with no header must never pass — that
// was the bug an empty configured key used to create.
func TestEndpoints_RejectBadInternalKey(t *testing.T) {
	handlers := map[string]func(*Feature, http.ResponseWriter, *http.Request){
		"lessons": (*Feature).GetLessons,
		"tenants": (*Feature).GetTenantsWithAIEnabled,
	}

	cases := []struct {
		name   string
		header string
		set    bool
	}{
		{"no header", "", false},
		{"empty header", "", true},
		{"wrong key", "nope", true},
	}

	for name, handler := range handlers {
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				f, mock, done := newTestFeature(t)
				defer done()

				req := httptest.NewRequest(http.MethodGet, "/?tenantId=t1", nil)
				if tc.set {
					req.Header.Set("x-internal-api-key", tc.header)
				}

				w := httptest.NewRecorder()
				handler(f, w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
				assert.Equal(t, "UNAUTHORIZED", bodyOf(t, w)["errorCode"])
				// No query was queued: rejection happens before any SQL.
				assert.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

// ---------- 2. GET /ai/lessons ----------

func TestGetLessons_RequiresTenantID(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLessons_UnknownTenant(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant" WHERE id`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons?tenantId=missing"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "LESSON_NOT_FOUND", bodyOf(t, w)["errorCode"])
}

// A tenant without the feature must not have its lessons listed at all.
func TestGetLessons_TenantWithoutAI(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant" WHERE id`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"aiEnabled"}).AddRow(false))

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons?tenantId=t1"))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "AI_NOT_ENABLED", bodyOf(t, w)["errorCode"])
	// The lessons query was never queued.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLessons_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant" WHERE id`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"aiEnabled"}).AddRow(true))
	mock.ExpectQuery(`FROM "Lesson" l`).
		WithArgs("t1", false).
		WillReturnRows(sqlmock.NewRows(lessonColumns()).
			AddRow("l1", "Aula 1", "aula-1", "video", "https://iframe.mediadelivery.net/x", nil, nil,
				true, "m1", "Módulo 1", "s1", "Seção 1", "c1", "Curso 1", "v1", "Vitrine 1"))

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons?tenantId=t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp aiLessonsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, "t1", resp.TenantID)
	assert.False(t, resp.OnlyUnprocessed)
	require.Len(t, resp.Lessons, 1)
	assert.Equal(t, "Vitrine 1", resp.Lessons[0].VitrineName)
	assert.True(t, resp.Lessons[0].TranscriptionCompleted)
	// NULL columns stay null rather than becoming "".
	assert.Nil(t, resp.Lessons[0].Thumbnail)
	assert.Nil(t, resp.Lessons[0].Content)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// onlyUnprocessed is bound as a query argument, and only the exact string
// "true" turns it on.
func TestGetLessons_OnlyUnprocessedFlag(t *testing.T) {
	cases := map[string]bool{
		"true":  true,
		"TRUE":  false,
		"1":     false,
		"false": false,
		"":      false,
	}

	for raw, want := range cases {
		t.Run("value="+raw, func(t *testing.T) {
			f, mock, done := newTestFeature(t)
			defer done()

			mock.ExpectQuery(`FROM "Tenant" WHERE id`).
				WillReturnRows(sqlmock.NewRows([]string{"aiEnabled"}).AddRow(true))
			mock.ExpectQuery(`FROM "Lesson" l`).
				WithArgs("t1", want).
				WillReturnRows(sqlmock.NewRows(lessonColumns()))

			w := httptest.NewRecorder()
			f.GetLessons(w, internalRequest("/lessons?tenantId=t1&onlyUnprocessed="+raw))

			require.Equal(t, http.StatusOK, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// An empty listing must serialise as [] rather than null.
func TestGetLessons_EmptyListIsArray(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"aiEnabled"}).AddRow(true))
	mock.ExpectQuery(`FROM "Lesson" l`).
		WillReturnRows(sqlmock.NewRows(lessonColumns()))

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons?tenantId=t1"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"lessons":[]`)
	assert.Contains(t, w.Body.String(), `"total":0`)
}

func TestGetLessons_QueryFailureIsFiveHundred(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"aiEnabled"}).AddRow(true))
	mock.ExpectQuery(`FROM "Lesson" l`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GetLessons(w, internalRequest("/lessons?tenantId=t1"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// ---------- 3. GET /ai/tenants ----------

func TestGetTenantsWithAIEnabled_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow("t1", "Cliente 1", true, "lib-1", "key-1").
			AddRow("t2", "Cliente 2", true, nil, nil))

	w := httptest.NewRecorder()
	f.GetTenantsWithAIEnabled(w, internalRequest("/tenants"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp aiTenantsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Tenants, 2)
	require.NotNil(t, resp.Tenants[0].BunnyLibraryID)
	assert.Equal(t, "lib-1", *resp.Tenants[0].BunnyLibraryID)
	assert.Nil(t, resp.Tenants[1].BunnyLibraryID)
	assert.Nil(t, resp.Tenants[1].BunnyLibraryApiKey)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTenantsWithAIEnabled_EmptyListIsArray(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}))

	w := httptest.NewRecorder()
	f.GetTenantsWithAIEnabled(w, internalRequest("/tenants"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"tenants":[]`)
}

func TestGetTenantsWithAIEnabled_QueryFailureIsFiveHundred(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.GetTenantsWithAIEnabled(w, internalRequest("/tenants"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

// ---------- 4. Method guards ----------

func TestHandlers_RejectNonGet(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	for name, handler := range map[string]func(*Feature, http.ResponseWriter, *http.Request){
		"lessons": (*Feature).GetLessons,
		"tenants": (*Feature).GetTenantsWithAIEnabled,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler(f, w, httptest.NewRequest(http.MethodPost, "/"+name, nil))
			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

// ---------- 5. Query shape ----------

// The listing is scoped by tenant and restricted to published, Bunny-hosted
// lessons.
func TestQueryLessons_FiltersAreInTheSQL(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`v\."tenantId" = \$1`).
		WithArgs("t1", true).
		WillReturnRows(sqlmock.NewRows(lessonColumns()))

	_, err := f.queryLessons(context.Background(), "t1", true)
	require.NoError(t, err)

	assert.Contains(t, sqlAILessons, "l.published = true")
	assert.Contains(t, sqlAILessons, "iframe.mediadelivery.net")
	assert.NoError(t, mock.ExpectationsWereMet())
}
