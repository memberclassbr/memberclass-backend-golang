package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// ---------- DTOs ----------

type processLessonsRequest struct {
	TenantID string `json:"tenantId"`
}

type processLessonsResponse struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	TenantID     string   `json:"tenantId"`
	LessonsCount int      `json:"lessonsCount"`
	JobIDs       []string `json:"jobIds,omitempty"`
}

// ---------- 1. HTTP handler ----------

// ProcessLessonsTenant handles `POST /api/v1/ai/tenants/process-lessons`.
// Body: { tenantId }. Behavior mirrors the legacy handler: validates the
// tenant has aiEnabled, lists unprocessed lessons, then INSERTs one
// VIDEO_PROCESSING job per lesson into the Railway pgvector jobs table
// and returns 202 with the list of jobIds. The worker pool picks them up
// asynchronously from there.
func (f *Feature) ProcessLessonsTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	var req processLessonsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCustomError(w, http.StatusBadRequest, "JSON inválido", "INVALID_REQUEST")
		return
	}
	if req.TenantID == "" {
		writeCustomError(w, http.StatusBadRequest, "tenantId é obrigatório", "MISSING_TENANT_ID")
		return
	}

	resp, status, err := f.enqueueLessonsForTenant(r.Context(), req.TenantID)
	if err != nil {
		writeError(w, status, http.StatusText(status), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) enqueueLessonsForTenant(ctx context.Context, tenantID string) (*processLessonsResponse, int, error) {
	// Reuse the same tenant lookup the pipeline uses so errors stay
	// consistent (404 vs 403 vs missing Bunny credentials).
	var (
		tID, tName              string
		aiEnabled               bool
		bunnyLibID, bunnyAPIKey *string
	)
	row := f.memberclassDB.QueryRowContext(ctx, sqlSelectTenantBunnyCreds, tenantID)
	if err := row.Scan(&tID, &tName, &aiEnabled, &bunnyLibID, &bunnyAPIKey); err != nil {
		// sql.ErrNoRows reads identically as a Scan error here; treat any
		// scan failure for an unknown tenant as 404.
		return nil, http.StatusNotFound, fmt.Errorf("tenant não encontrado")
	}
	if !aiEnabled {
		return nil, http.StatusForbidden, fmt.Errorf("IA não está habilitada para este tenant")
	}

	// List unprocessed lessons.
	rows, err := f.memberclassDB.QueryContext(ctx, sqlSelectUnprocessedLessons, tenantID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error listing lessons: %w", err)
	}
	defer rows.Close()

	type lessonRow struct {
		ID       string
		Name     string
		MediaURL string
		CourseID string
	}
	var lessons []lessonRow
	for rows.Next() {
		var (
			l                                                                                           lessonRow
			slug                                                                                        string
			lessonType, thumbnail, content                                                              *string
			moduleID, moduleName, sectionID, sectionName, courseName, vitrineID, vitrineName            string
		)
		if err := rows.Scan(
			&l.ID, &l.Name, &slug,
			&lessonType, &l.MediaURL, &thumbnail, &content,
			&moduleID, &moduleName, &sectionID, &sectionName,
			&l.CourseID, &courseName, &vitrineID, &vitrineName,
		); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("scan lesson: %w", err)
		}
		lessons = append(lessons, l)
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("iterate lessons: %w", err)
	}

	if len(lessons) == 0 {
		return &processLessonsResponse{
			Success:      false,
			Message:      "Nenhuma lesson não processada encontrada para este tenant",
			TenantID:     tenantID,
			LessonsCount: 0,
		}, http.StatusOK, nil
	}

	// Insert one job per lesson on the Railway pgvector jobs table.
	jobIDs := make([]string, 0, len(lessons))
	for _, l := range lessons {
		payload, err := json.Marshal(jobPayload{
			LessonID: l.ID,
			TenantID: tenantID,
			VideoURL: l.MediaURL,
			CourseID: l.CourseID,
			Title:    l.Name,
		})
		if err != nil {
			f.log.Error("transcription.enqueue.marshal_failed", "lesson", l.ID, "error", err.Error())
			continue
		}
		jobID := uuid.NewString()
		if _, err := f.transcriptionDB.ExecContext(ctx, sqlInsertJob,
			jobID, tenantID, 0, payload, 3,
		); err != nil {
			f.log.Error("transcription.enqueue.insert_failed",
				"tenant", tenantID, "lesson", l.ID, "error", err.Error())
			continue
		}
		jobIDs = append(jobIDs, jobID)
	}

	return &processLessonsResponse{
		Success:      true,
		Message:      "Jobs de transcrição enfileirados com sucesso",
		TenantID:     tenantID,
		LessonsCount: len(jobIDs),
		JobIDs:       jobIDs,
	}, http.StatusAccepted, nil
}
