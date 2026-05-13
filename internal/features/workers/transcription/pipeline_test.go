package transcription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/logger"
)

// fakeAudio writes a tiny non-empty MP3 fixture and returns it as the only
// audio part. Bypasses Bunny + ffmpeg entirely so the test can exercise
// the SQL + OpenAI portion of the pipeline.
func fakeAudio(t *testing.T) resolveAudioFunc {
	t.Helper()
	return func(ctx context.Context, libID, guid, accessKey, tmpDir string) ([]string, float64, error) {
		part := filepath.Join(tmpDir, "fake.mp3")
		if err := os.WriteFile(part, []byte("ID3FAKE"), 0o600); err != nil {
			t.Fatal(err)
		}
		return []string{part}, 5.0, nil
	}
}

// newFakeOpenAIForPipeline returns an httptest server that responds to
// both Whisper and Embeddings as the pipeline drives it through.
func newFakeOpenAIForPipeline(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(whisperResponse{
				Text:     "oi mundo",
				Language: "pt",
				Duration: 5,
				Segments: []whisperSegment{
					{Start: 0, End: 2.5, Text: "oi"},
					{Start: 2.5, End: 5, Text: "mundo"},
				},
			})
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(embeddingsResponse{
				Data:  []embedding{{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}}},
				Usage: usage{TotalTokens: 7},
			})
		default:
			t.Fatalf("unexpected OpenAI path: %s", r.URL.Path)
		}
	}))
}

func TestExecuteJob_HappyPath(t *testing.T) {
	transcriptionDB, txMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer transcriptionDB.Close()

	memberclassDB, mcMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer memberclassDB.Close()

	openai := newFakeOpenAIForPipeline(t)
	defer openai.Close()

	f := &Feature{
		transcriptionDB:      transcriptionDB,
		memberclassDB:        memberclassDB,
		log:                  logger.NewLogger(),
		openaiAPIKey:         "test-key",
		openaiBaseURL:        openai.URL,
		bunnyBaseURL:         "https://bunny.invalid",
		httpClient:           openai.Client(),
		testHookResolveAudio: fakeAudio(t),
	}

	tenantID := "tenant-abc"
	lessonID := "lesson-xyz"
	jobID := "job-123"

	// Tenant lookup returns aiEnabled + Bunny creds.
	mcMock.ExpectQuery(`SELECT id, name, "aiEnabled".*FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "Tenant Foo", true, "383534", "tenant-bunny-key"))

	// Transcription DB: BEGIN
	txMock.ExpectBegin()
	// UPSERT video — RETURNING id
	txMock.ExpectQuery(`INSERT INTO videos`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("video-uuid-1"))
	// DELETE chunks/transcripts (reprocess housekeeping; happens even on first-time)
	txMock.ExpectExec(`DELETE FROM chunks`).WithArgs("video-uuid-1").WillReturnResult(sqlmock.NewResult(0, 0))
	txMock.ExpectExec(`DELETE FROM transcripts`).WithArgs("video-uuid-1").WillReturnResult(sqlmock.NewResult(0, 0))
	// INSERT transcript
	txMock.ExpectExec(`INSERT INTO transcripts`).WillReturnResult(sqlmock.NewResult(0, 1))
	// CopyIn for chunks: a single chunk => one ExecContext to push the row
	// and one ExecContext to flush; both routed through the prepared stmt.
	prep := txMock.ExpectPrepare(`COPY "public"."chunks"`)
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`UPDATE videos`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`INSERT INTO token_usage`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectCommit()

	// Memberclass UPDATE Lesson
	mcMock.ExpectExec(`UPDATE "Lesson"`).
		WithArgs(lessonID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Mark job COMPLETED
	txMock.ExpectExec(`UPDATE jobs.*SET status.*COMPLETED`).WillReturnResult(sqlmock.NewResult(0, 1))

	payload, _ := json.Marshal(jobPayload{
		LessonID: lessonID,
		TenantID: tenantID,
		VideoURL: "https://iframe.mediadelivery.net/embed/383534/abc-guid-123?autoplay=false",
		Title:    "Aula 01",
	})

	if err := f.executeJob(context.Background(), jobID, tenantID, payload); err != nil {
		t.Fatalf("executeJob failed: %v", err)
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transcription DB expectations: %v", err)
	}
	if err := mcMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("memberclass DB expectations: %v", err)
	}
}

func TestExecuteJob_FailsWhenTenantAINotEnabled(t *testing.T) {
	transcriptionDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		log:             logger.NewLogger(),
		openaiAPIKey:    "test-key",
	}
	mcMock.ExpectQuery(`FROM "Tenant"`).
		WithArgs("t").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow("t", "n", false, "lib", "key"))

	payload, _ := json.Marshal(jobPayload{
		LessonID: "l", TenantID: "t",
		VideoURL: "https://iframe.mediadelivery.net/embed/lib/guid",
	})
	err := f.executeJob(context.Background(), "j", "t", payload)
	if err == nil || !strings.Contains(err.Error(), "aiEnabled=false") {
		t.Fatalf("want aiEnabled error, got %v", err)
	}
}

func TestExecuteJob_RejectsBadPayload(t *testing.T) {
	transcriptionDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	memberclassDB, _, _ := sqlmock.New()
	defer memberclassDB.Close()

	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		log:             logger.NewLogger(),
		openaiAPIKey:    "k",
	}
	if err := f.executeJob(context.Background(), "j", "t", []byte("{not json")); err == nil {
		t.Fatal("want decode error")
	}
	if err := f.executeJob(context.Background(), "j", "t", []byte(`{"lessonId":"l"}`)); err == nil {
		t.Fatal("want missing-videoUrl error")
	}
}

func TestPgvectorString(t *testing.T) {
	if got := pgvectorString(nil); got != "[]" {
		t.Fatalf("nil -> %q", got)
	}
	if got := pgvectorString([]float32{0.1, 0.25, -0.5}); !regexp.MustCompile(`^\[0\.1,0\.25,-0\.5\]$`).MatchString(got) {
		t.Fatalf("unexpected encoding: %q", got)
	}
}

