package transcription

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

// Register mounts the slice's HTTP routes on r. r is expected to already
// be scoped to `/api/v1/ai`; paths below are relative to that prefix.
//
// The slice owns four routes:
//   - POST   /tenants/process-lessons          enqueue a TRANSCRIPTION job per selected lesson
//   - GET    /jobs/{jobId}                     poll job status / result
//   - PATCH  /lessons/{lessonId}/transcription manually flip transcriptionCompleted (backwards compat)
//   - POST   /search                           RAG cosine-similarity search over chunks
//
// All four gate on x-internal-api-key matching INTERNAL_AI_API_KEY. The
// previous code path enforced this inline in each handler rather than via
// middleware; we keep the same surface so existing callers don't break.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Post("/tenants/process-lessons", f.ProcessLessonsTenant)
	r.Get("/jobs/{jobId}", f.GetJobStatus)
	r.Patch("/lessons/{lessonId}/transcription", f.UpdateLessonTranscription)
	r.Post("/search", f.Search)
}

// requireInternalAPIKey validates x-internal-api-key against the
// INTERNAL_AI_API_KEY env var. Returns false (and writes 401) when the
// caller is unauthorized — handlers should `return` immediately on false.
func (f *Feature) requireInternalAPIKey(w http.ResponseWriter, r *http.Request) bool {
	got := r.Header.Get("x-internal-api-key")
	want := os.Getenv("INTERNAL_AI_API_KEY")
	if want == "" || got != want {
		writeCustomError(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return false
	}
	return true
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
