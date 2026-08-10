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
	"github.com/memberclass-backend-golang/internal/platform/logger"
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

// newFakePandaServer models the real Panda API for executeJob's Panda path:
// player URLs carry the video_external_id, GET /videos maps it to the
// internal id, and only the internal id works against /subtitles. Hitting
// /subtitles with the external id returns the real 404 body, so the
// happy-path test FAILS if the resolution step is ever skipped.
func newFakePandaServer(t *testing.T, externalID, internalID, srclang, vtt string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"videos":[{"id":"` + internalID + `","video_external_id":"` + externalID + `"}],"pages":1,"total":1}`))
		case "/subtitles/" + externalID:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errCode":"NotFound","errMsg":"NotFound","detail":"Video not found"}`))
		case "/subtitles/" + internalID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"subtitles":[{"srclang":"` + srclang + `","label":"Português (BR)","hidden":false,"is_uploaded":true}]}`))
		case "/subtitles/" + internalID + "/" + srclang:
			_, _ = w.Write([]byte(vtt))
		default:
			t.Fatalf("unexpected panda path: %s", r.URL.Path)
		}
	}))
}

// newFakeOpenAIEmbeddingsOnly returns an httptest server that only serves
// /v1/embeddings — the Panda path never calls Whisper, so any hit on
// /v1/audio/transcriptions would indicate the dispatch took the wrong
// branch.
func newFakeOpenAIEmbeddingsOnly(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected OpenAI path on Panda path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data:  []embedding{{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}}},
			Usage: usage{TotalTokens: 7},
		})
	}))
}

func TestExecuteJob_PandaHappyPath(t *testing.T) {
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

	// The player URL carries the external id; only the internal id is valid
	// against /subtitles (the fake 404s the external id, like the real API).
	externalID := "18d69307-e9af-4d1a-8891-77f88794ce43"
	internalID := "4675da7f-cc57-4224-a66d-0e3b0b4abafc"
	vtt := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:03.000\n" +
		"Olá, bem-vindo ao vídeo.\n\n" +
		"00:00:03.000 --> 00:00:06.000\n" +
		"Vamos aprender Go.\n"
	panda := newFakePandaServer(t, externalID, internalID, "pt-BR", vtt)
	defer panda.Close()

	openai := newFakeOpenAIEmbeddingsOnly(t)
	defer openai.Close()

	tenantID := "tenant-panda"
	lessonID := "lesson-panda"
	jobID := "job-panda-1"

	f := &Feature{
		transcriptionDB:     transcriptionDB,
		memberclassDB:       memberclassDB,
		log:                 logger.NewLogger(),
		openaiAPIKey:        "test-key",
		openaiBaseURL:       openai.URL,
		httpClient:          openai.Client(),
		pandaBaseURL:        panda.URL,
		pandaAPIKey:         "panda-test-key",
		pandaAllowedTenants: map[string]bool{tenantID: true},
	}

	// Tenant lookup: Panda tenants don't need Bunny creds, so these columns
	// come back NULL.
	mcMock.ExpectQuery(`SELECT id, name, "aiEnabled".*FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "Tenant Panda", true, nil, nil))

	txMock.ExpectBegin()
	txMock.ExpectQuery(`INSERT INTO videos`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("video-uuid-panda"))
	txMock.ExpectExec(`DELETE FROM chunks`).WithArgs("video-uuid-panda").WillReturnResult(sqlmock.NewResult(0, 0))
	txMock.ExpectExec(`DELETE FROM transcripts`).WithArgs("video-uuid-panda").WillReturnResult(sqlmock.NewResult(0, 0))
	txMock.ExpectExec(`INSERT INTO transcripts`).WillReturnResult(sqlmock.NewResult(0, 1))
	prep := txMock.ExpectPrepare(`COPY "public"."chunks"`)
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`UPDATE videos`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`INSERT INTO token_usage`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectCommit()

	mcMock.ExpectExec(`UPDATE "Lesson"`).
		WithArgs(lessonID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	txMock.ExpectExec(`UPDATE jobs.*SET status.*COMPLETED`).WillReturnResult(sqlmock.NewResult(0, 1))

	payload, _ := json.Marshal(jobPayload{
		LessonID: lessonID,
		TenantID: tenantID,
		VideoURL: "https://player-vz-test.tv.pandavideo.com.br/embed/?v=" + externalID,
		Title:    "Aula Panda",
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

func TestExecuteJob_PandaTenantNotAllowlisted(t *testing.T) {
	transcriptionDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	tenantID := "tenant-not-allowed"
	f := &Feature{
		transcriptionDB: transcriptionDB,
		memberclassDB:   memberclassDB,
		log:             logger.NewLogger(),
		openaiAPIKey:    "test-key",
		pandaAPIKey:     "panda-key",
		// pandaAllowedTenants intentionally empty/nil: tenant not allowlisted.
	}

	mcMock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "T", true, nil, nil))

	payload, _ := json.Marshal(jobPayload{
		LessonID: "l", TenantID: tenantID,
		VideoURL: "https://player-vz-test.tv.pandavideo.com.br/embed/?v=abc-123",
	})
	err := f.executeJob(context.Background(), "j", tenantID, payload)
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("want 'not enabled' error, got %v", err)
	}
}

func TestExecuteJob_PandaNoSubtitleTracks(t *testing.T) {
	transcriptionDB, _, _ := sqlmock.New()
	defer transcriptionDB.Close()
	memberclassDB, mcMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer memberclassDB.Close()

	externalID := "18d69307-e9af-4d1a-8891-77f88794ce43"
	internalID := "4675da7f-cc57-4224-a66d-0e3b0b4abafc"
	panda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/videos":
			_, _ = w.Write([]byte(`{"videos":[{"id":"` + internalID + `","video_external_id":"` + externalID + `"}],"pages":1,"total":1}`))
		case "/subtitles/" + externalID:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errCode":"NotFound","errMsg":"NotFound","detail":"Video not found"}`))
		case "/subtitles/" + internalID:
			_, _ = w.Write([]byte(`{"subtitles":[]}`))
		default:
			t.Fatalf("unexpected panda path: %s", r.URL.Path)
		}
	}))
	defer panda.Close()

	tenantID := "tenant-panda-2"
	f := &Feature{
		transcriptionDB:     transcriptionDB,
		memberclassDB:       memberclassDB,
		log:                 logger.NewLogger(),
		openaiAPIKey:        "test-key",
		pandaBaseURL:        panda.URL,
		pandaAPIKey:         "panda-key",
		httpClient:          panda.Client(),
		pandaAllowedTenants: map[string]bool{tenantID: true},
	}

	mcMock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "T", true, nil, nil))

	payload, _ := json.Marshal(jobPayload{
		LessonID: "l", TenantID: tenantID,
		VideoURL: "https://player-vz-test.tv.pandavideo.com.br/embed/?v=" + externalID,
	})
	err := f.executeJob(context.Background(), "j", tenantID, payload)
	if err == nil || !strings.Contains(err.Error(), "dashboard") {
		t.Fatalf("want dashboard error, got %v", err)
	}
}

// TestExecuteJob_BunnyURLStillUsesAudioWhisperPath is a regression guard for
// the Step 7 dispatch refactor: a Bunny-URL job must take the audio/Whisper
// path (testHookResolveAudio invoked) even when the SAME tenant is also
// Panda-allowlisted — the source dispatch is decided by the URL, not by
// tenant configuration.
func TestExecuteJob_BunnyURLStillUsesAudioWhisperPath(t *testing.T) {
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

	hookInvoked := false
	hook := func(ctx context.Context, libID, guid, accessKey, tmpDir string) ([]string, float64, error) {
		hookInvoked = true
		part := filepath.Join(tmpDir, "fake.mp3")
		if err := os.WriteFile(part, []byte("ID3FAKE"), 0o600); err != nil {
			t.Fatal(err)
		}
		return []string{part}, 5.0, nil
	}

	tenantID := "tenant-abc"
	lessonID := "lesson-xyz"
	jobID := "job-999"

	f := &Feature{
		transcriptionDB:      transcriptionDB,
		memberclassDB:        memberclassDB,
		log:                  logger.NewLogger(),
		openaiAPIKey:         "test-key",
		openaiBaseURL:        openai.URL,
		bunnyBaseURL:         "https://bunny.invalid",
		httpClient:           openai.Client(),
		testHookResolveAudio: hook,
		// Panda enabled for this same tenant — must NOT affect a Bunny URL.
		pandaAllowedTenants: map[string]bool{tenantID: true},
		pandaAPIKey:         "panda-key",
	}

	mcMock.ExpectQuery(`SELECT id, name, "aiEnabled".*FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(tenantID, "Tenant Foo", true, "383534", "tenant-bunny-key"))

	txMock.ExpectBegin()
	txMock.ExpectQuery(`INSERT INTO videos`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("video-uuid-2"))
	txMock.ExpectExec(`DELETE FROM chunks`).WithArgs("video-uuid-2").WillReturnResult(sqlmock.NewResult(0, 0))
	txMock.ExpectExec(`DELETE FROM transcripts`).WithArgs("video-uuid-2").WillReturnResult(sqlmock.NewResult(0, 0))
	txMock.ExpectExec(`INSERT INTO transcripts`).WillReturnResult(sqlmock.NewResult(0, 1))
	prep := txMock.ExpectPrepare(`COPY "public"."chunks"`)
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`UPDATE videos`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectExec(`INSERT INTO token_usage`).WillReturnResult(sqlmock.NewResult(0, 1))
	txMock.ExpectCommit()

	mcMock.ExpectExec(`UPDATE "Lesson"`).
		WithArgs(lessonID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	txMock.ExpectExec(`UPDATE jobs.*SET status.*COMPLETED`).WillReturnResult(sqlmock.NewResult(0, 1))

	payload, _ := json.Marshal(jobPayload{
		LessonID: lessonID,
		TenantID: tenantID,
		VideoURL: "https://iframe.mediadelivery.net/embed/383534/abc-guid-123",
		Title:    "Aula 01",
	})

	if err := f.executeJob(context.Background(), jobID, tenantID, payload); err != nil {
		t.Fatalf("executeJob failed: %v", err)
	}
	if !hookInvoked {
		t.Fatal("expected testHookResolveAudio to be invoked for a Bunny URL job")
	}
	if err := txMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transcription DB expectations: %v", err)
	}
	if err := mcMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("memberclass DB expectations: %v", err)
	}
}

