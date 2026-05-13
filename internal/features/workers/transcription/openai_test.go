package transcription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newFakeOpenAI(t *testing.T, handler http.HandlerFunc) *Feature {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Feature{
		openaiAPIKey:  "test-key",
		openaiBaseURL: server.URL,
		httpClient:    server.Client(),
	}
}

func TestEmbedBatch_HappyPath(t *testing.T) {
	f := newFakeOpenAI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing/wrong auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		// Echo back two embeddings in REVERSED order to prove we honor `index`.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data: []embedding{
				{Index: 1, Embedding: []float32{0.3, 0.4}},
				{Index: 0, Embedding: []float32{0.1, 0.2}},
			},
			Usage: usage{PromptTokens: 10, TotalTokens: 10},
		})
	})

	vecs, tokens, err := f.embedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 10 {
		t.Fatalf("tokens=%d, want 10", tokens)
	}
	if len(vecs) != 2 {
		t.Fatalf("vecs len=%d", len(vecs))
	}
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.3 {
		t.Fatalf("vectors not aligned by index: %+v", vecs)
	}
}

func TestEmbedBatch_PropagatesHTTPError(t *testing.T) {
	f := newFakeOpenAI(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	})
	_, _, err := f.embedBatch(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("want error on 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error missing status: %v", err)
	}
}

func TestEmbedBatch_RejectsMissingVector(t *testing.T) {
	f := newFakeOpenAI(t, func(w http.ResponseWriter, r *http.Request) {
		// Return only one embedding for two inputs — should fail.
		_ = json.NewEncoder(w).Encode(embeddingsResponse{
			Data:  []embedding{{Index: 0, Embedding: []float32{0.1}}},
			Usage: usage{TotalTokens: 5},
		})
	})
	_, _, err := f.embedBatch(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("want error on missing vector")
	}
}

func TestTranscribeAudio_HappyPath(t *testing.T) {
	f := newFakeOpenAI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("expected multipart, got %q", r.Header.Get("Content-Type"))
		}
		// Sanity-check the form fields the slice sets — Whisper rejects
		// requests without `model`, so a regression here ships broken
		// transcription.
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if r.FormValue("model") != whisperModel {
			t.Fatalf("model=%q", r.FormValue("model"))
		}
		if r.FormValue("response_format") != "verbose_json" {
			t.Fatalf("response_format=%q", r.FormValue("response_format"))
		}
		if r.FormValue("language") != "pt" {
			t.Fatalf("language=%q", r.FormValue("language"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(whisperResponse{
			Text:     "oi mundo",
			Language: "pt",
			Duration: 5.0,
			Segments: []whisperSegment{
				{Start: 0, End: 2.5, Text: "oi"},
				{Start: 2.5, End: 5, Text: "mundo"},
			},
		})
	})

	resp, err := f.transcribeAudio(context.Background(), strings.NewReader("FAKE-MP3-DATA"), "audio.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "oi mundo" || len(resp.Segments) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestTranscribeAudio_PropagatesHTTPError(t *testing.T) {
	f := newFakeOpenAI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Error(w, "audio too large", http.StatusRequestEntityTooLarge)
	})
	_, err := f.transcribeAudio(context.Background(), strings.NewReader("data"), "x.mp3")
	if err == nil {
		t.Fatal("want error on 413")
	}
}
