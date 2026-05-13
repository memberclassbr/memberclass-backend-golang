package transcription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func TestGetTranscriptionStats_Requires401WithoutKey(t *testing.T) {
	setEnvKey(t, "secret")
	f := &Feature{log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodGet, "/transcription-stats?tenantId=t", nil)
	w := httptest.NewRecorder()
	f.GetTranscriptionStats(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetTranscriptionStats_RejectsMissingTenant(t *testing.T) {
	setEnvKey(t, "k")
	memberclassDB, _, _ := sqlmock.New()
	defer memberclassDB.Close()
	f := &Feature{memberclassDB: memberclassDB, log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodGet, "/transcription-stats", nil)
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.GetTranscriptionStats(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetTranscriptionStats_TenantScope(t *testing.T) {
	setEnvKey(t, "k")
	memberclassDB, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	mock.ExpectQuery(`FROM "Lesson"`).
		WithArgs("t-1", "", "").
		WillReturnRows(sqlmock.NewRows([]string{"total", "transcribed", "pending"}).
			AddRow(50, 12, 38))

	f := &Feature{memberclassDB: memberclassDB, log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodGet, "/transcription-stats?tenantId=t-1", nil)
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.GetTranscriptionStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp transcriptionStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 50 || resp.Transcribed != 12 || resp.Pending != 38 {
		t.Fatalf("got %+v", resp)
	}
}

func TestGetTranscriptionStats_CourseAndModuleFilters(t *testing.T) {
	setEnvKey(t, "k")
	memberclassDB, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	mock.ExpectQuery(`FROM "Lesson"`).
		WithArgs("t-1", "course-9", "module-2").
		WillReturnRows(sqlmock.NewRows([]string{"total", "transcribed", "pending"}).
			AddRow(8, 8, 0))

	f := &Feature{memberclassDB: memberclassDB, log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodGet, "/transcription-stats?tenantId=t-1&courseId=course-9&moduleId=module-2", nil)
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.GetTranscriptionStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp transcriptionStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 8 || resp.Transcribed != 8 || resp.Pending != 0 {
		t.Fatalf("got %+v", resp)
	}
	if resp.CourseID != "course-9" || resp.ModuleID != "module-2" {
		t.Fatalf("scope echo wrong: %+v", resp)
	}
}
