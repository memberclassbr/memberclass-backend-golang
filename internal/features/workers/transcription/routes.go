package transcription

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

// Register mounts the slice's HTTP routes on r. r is expected to already
// be scoped to `/api/v1/ai`; paths below are relative to that prefix.
//
// The slice owns five routes:
//   - POST   /tenants/process-lessons          enqueue a TRANSCRIPTION job per selected (or all unprocessed) lesson
//   - GET    /jobs/{jobId}                     poll job status / result
//   - PATCH  /lessons/{lessonId}/transcription manually flip transcriptionCompleted (backwards compat)
//   - POST   /search                           RAG cosine-similarity search over chunks
//   - GET    /transcription-stats             { total, transcribed, pending } per scope
//
// All five gate on x-internal-api-key matching INTERNAL_AI_API_KEY. The
// previous code path enforced this inline in each handler rather than via
// middleware; we keep the same surface so existing callers don't break.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Post("/tenants/process-lessons", f.ProcessLessonsTenant)
	r.Get("/jobs/{jobId}", f.GetJobStatus)
	r.Patch("/lessons/{lessonId}/transcription", f.UpdateLessonTranscription)
	r.Post("/search", f.Search)
	r.Get("/transcription-stats", f.GetTranscriptionStats)
}

// requireInternalAPIKey validates x-internal-api-key against the
// INTERNAL_AI_API_KEY env var. Returns false (and writes 401) when the
// caller is unauthorized — handlers should `return` immediately on false.
//
// Uses crypto/subtle.ConstantTimeCompare to avoid leaking the key one
// byte at a time through response-timing differences. The empty-want
// short-circuit is fine because an attacker can't influence it; we only
// need constant-time comparison once both sides are non-empty.
func (f *Feature) requireInternalAPIKey(w http.ResponseWriter, r *http.Request) bool {
	got := r.Header.Get("x-internal-api-key")
	want := os.Getenv("INTERNAL_AI_API_KEY")
	if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		writeCustomError(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return false
	}
	return true
}

// maxRequestBodyBytes caps the size of any JSON body the slice's handlers
// will read. The largest legitimate payload is the LessonIDs array sent to
// process-lessons (a few hundred CUIDs ~= a few KB). 1 MB leaves ample
// headroom while preventing OOM-style DoS from a misbehaving — or
// compromised — admin caller.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// maxLessonIDsPerRequest bounds the number of lessons a single enqueue
// call can target. The admin UI ships a hand-picked list; multi-thousand
// arrays signal either a bug or abuse.
const maxLessonIDsPerRequest = 1000

// limitBody wraps r.Body with http.MaxBytesReader so subsequent decodes
// bail out with an error instead of allocating unbounded memory.
func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
}

// ---------- HTTP helpers (kept local — the slice mirrors the legacy
// dto.ErrorResponse shape so existing clients keep parsing the same fields).

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, errText, message string) {
	writeJSON(w, code, map[string]any{
		"error":   errText,
		"message": message,
	})
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
