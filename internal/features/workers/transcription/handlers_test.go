package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func setEnvKey(t *testing.T, val string) { t.Helper(); t.Setenv("INTERNAL_AI_API_KEY", val) }

func TestProcessLessonsTenant_Requires401WithoutKey(t *testing.T) {
	setEnvKey(t, "secret")
	f := &Feature{log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodPost, "/tenants/process-lessons", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.ProcessLessonsTenant(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestProcessLessonsTenant_MissingTenantID(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, _, _ := sqlmock.New()
	memberclassDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	defer memberclassDB.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		openaiAPIKey:    "x",
		log:             logger.NewLogger(),
	}
	req := httptest.NewRequest(http.MethodPost, "/tenants/process-lessons", strings.NewReader(`{}`))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.ProcessLessonsTenant(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestProcessLessonsTenant_EmptyLessonIDsEnqueuesAllUnprocessed(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()
	defer memberclassDB.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		openaiAPIKey:    "x",
		log:             logger.NewLogger(),
	}

	tenantID := "tenant-fallback"
	mcMock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "T", true, "lib", "key"))

	// Unprocessed lookup returns 2 lessons; both should be enqueued.
	mcMock.ExpectQuery(`FROM "Lesson"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "type", "mediaUrl", "thumbnail", "content",
			"module_id", "module_name", "section_id", "section_name",
			"course_id", "course_name", "vitrine_id", "vitrine_name",
		}).
			AddRow("l1", "Aula 1", "aula-1", nil,
				"https://iframe.mediadelivery.net/embed/lib/g1", nil, nil,
				"m1", "Mod", "s1", "Sec", "c1", "Curso", "v1", "Vit").
			AddRow("l2", "Aula 2", "aula-2", nil,
				"https://iframe.mediadelivery.net/embed/lib/g2", nil, nil,
				"m1", "Mod", "s1", "Sec", "c1", "Curso", "v1", "Vit"))

	txMock.ExpectExec(`INSERT INTO jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`INSERT INTO jobs`).WillReturnResult(sqlmock.NewResult(0, 1))

	body, _ := json.Marshal(processLessonsRequest{TenantID: tenantID})
	req := httptest.NewRequest(http.MethodPost, "/tenants/process-lessons", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.ProcessLessonsTenant(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp processLessonsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.EnqueuedCount != 2 || len(resp.Enqueued) != 2 {
		t.Fatalf("unexpected: %+v", resp)
	}
	if err := mcMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessLessonsTenant_EnqueuesSelectedLessons(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()
	defer memberclassDB.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		openaiAPIKey:    "x",
		log:             logger.NewLogger(),
	}

	tenantID := "tenant-1"
	mcMock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "T", true, "lib", "key"))

	// Two requested lessons. Mock returns:
	//   l1 — eligible (transcriptionCompleted=false, Bunny mediaUrl)
	//   l2 — already transcribed (skipped reason: already transcribed)
	// l3 (third id in request) is not returned by the query -> skipped reason: not found.
	mcMock.ExpectQuery(`FROM "Lesson"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "type", "mediaUrl", "thumbnail", "content",
			"module_id", "module_name", "section_id", "section_name",
			"course_id", "course_name", "vitrine_id", "vitrine_name",
			"transcription_completed",
		}).AddRow(
			"l1", "Aula 1", "aula-1", nil,
			"https://iframe.mediadelivery.net/embed/lib/g1?x=1",
			nil, nil,
			"m1", "Mod", "s1", "Sec", "c1", "Curso", "v1", "Vit",
			false,
		).AddRow(
			"l2", "Aula 2", "aula-2", nil,
			"https://iframe.mediadelivery.net/embed/lib/g2",
			nil, nil,
			"m1", "Mod", "s1", "Sec", "c1", "Curso", "v1", "Vit",
			true, // already transcribed -> skipped
		))

	// Only l1 produces an INSERT INTO jobs.
	txMock.ExpectExec(`INSERT INTO jobs`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body, _ := json.Marshal(processLessonsRequest{
		TenantID:  tenantID,
		LessonIDs: []string{"l1", "l2", "l3"},
	})
	req := httptest.NewRequest(http.MethodPost, "/tenants/process-lessons", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.ProcessLessonsTenant(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp processLessonsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.EnqueuedCount != 1 || len(resp.Enqueued) != 1 || resp.Enqueued[0].LessonID != "l1" {
		t.Fatalf("unexpected enqueued: %+v", resp)
	}
	if len(resp.Skipped) != 2 {
		t.Fatalf("expected 2 skipped (l2 transcribed, l3 not found), got %+v", resp.Skipped)
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := mcMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetJobStatus_NotFound(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()
	f := &Feature{
		transcriptionDB: transcriptionDB,
		openaiAPIKey:    "x",
		log:             logger.NewLogger(),
	}
	mock.ExpectQuery(`FROM jobs`).WithArgs("missing").WillReturnRows(sqlmock.NewRows([]string{
		"id", "tenant_id", "status", "attempts", "error", "started_at", "completed_at", "payload", "result",
	}))

	r := chi.NewRouter()
	r.Get("/jobs/{jobId}", f.GetJobStatus)
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateLessonTranscription_FlipsFlag(t *testing.T) {
	setEnvKey(t, "k")
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()
	f := &Feature{memberclassDB: memberclassDB, log: logger.NewLogger()}

	mcMock.ExpectExec(`UPDATE "Lesson"`).
		WithArgs("l1", true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := chi.NewRouter()
	r.Patch("/lessons/{lessonId}/transcription", f.UpdateLessonTranscription)
	body, _ := json.Marshal(updateLessonTranscriptionRequest{TranscriptionCompleted: ptrBool(true)})
	req := httptest.NewRequest(http.MethodPatch, "/lessons/l1/transcription", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func ptrBool(b bool) *bool { return &b }

// Compile-time check: Feature satisfies http.Handler-style method signatures.
var (
	_ context.Context
	_ http.HandlerFunc = (&Feature{}).ProcessLessonsTenant
	_ http.HandlerFunc = (&Feature{}).GetJobStatus
	_ http.HandlerFunc = (&Feature{}).UpdateLessonTranscription
)
