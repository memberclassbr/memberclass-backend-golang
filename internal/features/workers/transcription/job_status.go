package transcription

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type jobStatusResponse struct {
	JobID       string          `json:"jobId"`
	TenantID    string          `json:"tenantId"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	Error       *string         `json:"error,omitempty"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// GetJobStatus handles `GET /api/v1/ai/jobs/{jobId}`. Returns the row
// from the Railway pgvector jobs table so the caller can poll for
// COMPLETED / FAILED.
func (f *Feature) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), "")
		return
	}
	if !f.requireInternalAPIKey(w, r) {
		return
	}
	if err := f.preflight(); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		writeCustomError(w, http.StatusBadRequest, "jobId é obrigatório", "MISSING_JOB_ID")
		return
	}

	var (
		resp         jobStatusResponse
		jobErr       sql.NullString
		startedAt    sql.NullTime
		completedAt  sql.NullTime
		payload      []byte
		result       []byte
	)
	row := f.transcriptionDB.QueryRowContext(r.Context(), sqlGetJobStatus, jobID)
	if err := row.Scan(
		&resp.JobID, &resp.TenantID, &resp.Status, &resp.Attempts,
		&jobErr, &startedAt, &completedAt, &payload, &result,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeCustomError(w, http.StatusNotFound, "Job não encontrado", "JOB_NOT_FOUND")
			return
		}
		f.log.Error("transcription.job_status.scan_failed", "jobId", jobID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "Internal Server Error", "")
		return
	}
	if jobErr.Valid {
		s := jobErr.String
		resp.Error = &s
	}
	if startedAt.Valid {
		t := startedAt.Time
		resp.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		resp.CompletedAt = &t
	}
	if len(payload) > 0 {
		resp.Payload = payload
	}
	if len(result) > 0 {
		resp.Result = result
	}

	writeJSON(w, http.StatusOK, resp)
}
