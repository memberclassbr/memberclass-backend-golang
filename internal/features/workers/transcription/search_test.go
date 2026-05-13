package transcription

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

func TestSearch_Requires401WithoutKey(t *testing.T) {
	setEnvKey(t, "secret")
	f := &Feature{log: logger.NewLogger()}
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.Search(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSearch_RejectsMissingQuery(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	f := &Feature{
		transcriptionDB: transcriptionDB,
		openaiAPIKey:    "x",
		log:             logger.NewLogger(),
	}
	body, _ := json.Marshal(searchRequest{TenantID: "t", Query: ""})
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.Search(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "MISSING_QUERY") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSearch_HappyPath_TenantScopeOnly(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()

	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data:  []embedding{{Index: 0, Embedding: []float32{0.1, 0.2}}},
			Usage: usage{TotalTokens: 5},
		})
	}))
	defer openai.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		openaiAPIKey:    "k",
		openaiBaseURL:   openai.URL,
		httpClient:      openai.Client(),
		log:             logger.NewLogger(),
	}

	txMock.ExpectQuery(`SELECT id, lesson_id, course_id, video_id`).
		WithArgs("[0.1,0.2]", "t-1", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "lesson_id", "course_id", "video_id", "text", "start_time", "end_time", "similarity"}).
			AddRow("chunk-1", "lesson-1", "course-1", "video-1", "Texto exemplo", 0.0, 12.5, 0.87))

	body, _ := json.Marshal(searchRequest{TenantID: "t-1", Query: "como funciona X"})
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp searchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 || resp.Hits[0].ChunkID != "chunk-1" || resp.Hits[0].Similarity != 0.87 {
		t.Fatalf("unexpected: %+v", resp)
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSearch_LessonScopeFiltersDirectly(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()

	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data:  []embedding{{Index: 0, Embedding: []float32{0.5}}},
			Usage: usage{TotalTokens: 1},
		})
	}))
	defer openai.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		openaiAPIKey:    "k",
		openaiBaseURL:   openai.URL,
		httpClient:      openai.Client(),
		log:             logger.NewLogger(),
	}

	txMock.ExpectQuery(`AND lesson_id = \$3`).
		WithArgs("[0.5]", "t-1", "lesson-X", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "lesson_id", "course_id", "video_id", "text", "start_time", "end_time", "similarity"}))

	body, _ := json.Marshal(searchRequest{
		TenantID: "t-1",
		Query:    "x",
		Scope:    searchScope{LessonID: "lesson-X"},
	})
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSearch_ModuleScopeResolvesViaMemberclass(t *testing.T) {
	setEnvKey(t, "k")
	transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer transcriptionDB.Close()
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data:  []embedding{{Index: 0, Embedding: []float32{0.1}}},
			Usage: usage{TotalTokens: 1},
		})
	}))
	defer openai.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		openaiAPIKey:    "k",
		openaiBaseURL:   openai.URL,
		httpClient:      openai.Client(),
		log:             logger.NewLogger(),
	}

	// Memberclass: moduleId → lessonIds.
	mcMock.ExpectQuery(`FROM "Lesson" WHERE "moduleId"`).
		WithArgs("module-9").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("L1").AddRow("L2"))

	// Transcription: chunk search filtered by lesson_id = ANY(...).
	txMock.ExpectQuery(`AND lesson_id = ANY\(\$3\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "lesson_id", "course_id", "video_id", "text", "start_time", "end_time", "similarity"}))

	body, _ := json.Marshal(searchRequest{
		TenantID: "t",
		Query:    "x",
		Scope:    searchScope{ModuleID: "module-9"},
	})
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(body))
	req.Header.Set("x-internal-api-key", "k")
	w := httptest.NewRecorder()
	f.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mcMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSearch_LimitDefaultsAndClamps(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero defaults to 10", 0, 10},
		{"negative defaults to 10", -5, 10},
		{"valid passthrough", 7, 7},
		{"too large clamps to 20", 100, 20},
		{"exactly 20", 20, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvKey(t, "k")
			transcriptionDB, txMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			defer transcriptionDB.Close()
			openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(embeddingsResponse{
					Data:  []embedding{{Index: 0, Embedding: []float32{0.1}}},
					Usage: usage{TotalTokens: 1},
				})
			}))
			defer openai.Close()

			f := &Feature{
				transcriptionDB: transcriptionDB,
				openaiAPIKey:    "k",
				openaiBaseURL:   openai.URL,
				httpClient:      openai.Client(),
				log:             logger.NewLogger(),
			}
			txMock.ExpectQuery(`SELECT`).
				WithArgs("[0.1]", "t", tt.want).
				WillReturnRows(sqlmock.NewRows([]string{"id", "lesson_id", "course_id", "video_id", "text", "start_time", "end_time", "similarity"}))

			body, _ := json.Marshal(searchRequest{TenantID: "t", Query: "x", Limit: tt.input})
			req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(body))
			req.Header.Set("x-internal-api-key", "k")
			w := httptest.NewRecorder()
			f.Search(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if err := txMock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
