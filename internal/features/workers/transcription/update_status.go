package transcription

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// updateLessonTranscriptionRequest mirrors the legacy DTO shape so existing
// callers don't break when the route moves into this slice.
type updateLessonTranscriptionRequest struct {
	TranscriptionCompleted *bool `json:"transcriptionCompleted"`
}

type updateLessonTranscriptionResponse struct {
	Lesson struct {
		ID                     string `json:"id"`
		TranscriptionCompleted bool   `json:"transcriptionCompleted"`
	} `json:"lesson"`
	Message string `json:"message"`
}

// UpdateLessonTranscription handles `PATCH /api/v1/ai/lessons/{lessonId}/transcription`.
// Kept for backwards compatibility — the legacy external service used this
// route to flip the flag after it finished work. With the in-process
// pipeline, executeJob does the flip itself; we still expose the route so
// operators can manually toggle a lesson without re-running the pipeline.
func (f *Feature) UpdateLessonTranscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), "")
		return
	}
	if !f.requireInternalAPIKey(w, r) {
		return
	}

	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		writeCustomError(w, http.StatusBadRequest, "lessonId é obrigatório", "MISSING_LESSON_ID")
		return
	}

	limitBody(w, r)
	var req updateLessonTranscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCustomError(w, http.StatusBadRequest, "JSON inválido", "INVALID_REQUEST")
		return
	}
	if req.TranscriptionCompleted == nil {
		writeCustomError(w, http.StatusBadRequest,
			"transcriptionCompleted deve ser um booleano", "INVALID_REQUEST")
		return
	}

	if _, err := f.memberclassDB.ExecContext(r.Context(),
		sqlMarkLessonTranscriptionStatus, lessonID, *req.TranscriptionCompleted,
	); err != nil {
		f.log.Error("transcription.update_status.exec_failed",
			"lessonId", lessonID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "Internal Server Error",
			"error updating transcription status")
		return
	}

	resp := updateLessonTranscriptionResponse{
		Message: "Status de transcrição atualizado com sucesso",
	}
	resp.Lesson.ID = lessonID
	resp.Lesson.TranscriptionCompleted = *req.TranscriptionCompleted
	writeJSON(w, http.StatusOK, resp)
}
