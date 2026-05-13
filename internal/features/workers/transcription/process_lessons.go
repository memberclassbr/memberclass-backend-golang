package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ---------- DTOs ----------

// processLessonsRequest is what the internal admin UI POSTs to start
// transcription. Two modes:
//
//   - lessonIds populated: enqueue only those (used when the admin
//     picked specific rows). Each id is validated against the tenant
//     and surfaced in `enqueued` or `skipped` with a reason.
//   - lessonIds empty: enqueue ALL Bunny-backed, unprocessed lessons
//     for the tenant. Admin UI uses this for "transcrever todas as
//     pendentes" buttons.
type processLessonsRequest struct {
	TenantID  string   `json:"tenantId"`
	LessonIDs []string `json:"lessonIds,omitempty"`
}

type enqueuedLesson struct {
	LessonID string `json:"lessonId"`
	JobID    string `json:"jobId"`
}

type skippedLesson struct {
	LessonID string `json:"lessonId"`
	Reason   string `json:"reason"`
}

type processLessonsResponse struct {
	Success      bool             `json:"success"`
	Message      string           `json:"message"`
	TenantID     string           `json:"tenantId"`
	Enqueued     []enqueuedLesson `json:"enqueued"`
	Skipped      []skippedLesson  `json:"skipped,omitempty"`
	EnqueuedCount int             `json:"enqueuedCount"`
}

// ---------- 1. HTTP handler ----------

// ProcessLessonsTenant handles `POST /api/v1/ai/tenants/process-lessons`.
//
// Body: { tenantId, lessonIds: []string }
//
// Behavior:
//   - Validates the tenant exists and has aiEnabled.
//   - For each lessonId, looks up the row (joined through Vitrine for
//     tenant ownership) and either enqueues a VIDEO_PROCESSING job on
//     the Railway pgvector jobs table OR records a `skipped` entry
//     with a reason (wrong tenant, already transcribed, non-Bunny URL,
//     unknown id).
//   - Returns 202 with the split lists so the admin UI can surface a
//     summary instead of guessing.
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

	var (
		resp   *processLessonsResponse
		status int
		err    error
	)
	if len(req.LessonIDs) == 0 {
		resp, status, err = f.enqueueAllUnprocessed(r.Context(), req.TenantID)
	} else {
		resp, status, err = f.enqueueSelectedLessons(r.Context(), req.TenantID, req.LessonIDs)
	}
	if err != nil {
		writeError(w, status, http.StatusText(status), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// ---------- 2. Business rule ----------

// enqueueAllUnprocessed inserts one VIDEO_PROCESSING job per Bunny-
// backed lesson under the tenant whose transcriptionCompleted flag is
// still false. Used by the admin UI's "transcribe all pending" action.
// Returns the same response shape as the selected-lessons path so the
// frontend can render a single summary table either way.
func (f *Feature) enqueueAllUnprocessed(ctx context.Context, tenantID string) (*processLessonsResponse, int, error) {
	var (
		tID, tName              string
		aiEnabled               bool
		bunnyLibID, bunnyAPIKey *string
	)
	row := f.memberclassDB.QueryRowContext(ctx, sqlSelectTenantBunnyCreds, tenantID)
	if err := row.Scan(&tID, &tName, &aiEnabled, &bunnyLibID, &bunnyAPIKey); err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("tenant não encontrado")
	}
	if !aiEnabled {
		return nil, http.StatusForbidden, fmt.Errorf("IA não está habilitada para este tenant")
	}

	rows, err := f.memberclassDB.QueryContext(ctx, sqlSelectUnprocessedLessons, tenantID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("listar lessons pendentes: %w", err)
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
			l                                                                                  lessonRow
			slug                                                                               string
			lessonType, thumbnail, content                                                     *string
			moduleID, moduleName, sectionID, sectionName, courseName, vitrineID, vitrineName   string
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

	resp := &processLessonsResponse{
		TenantID: tenantID,
		Enqueued: make([]enqueuedLesson, 0, len(lessons)),
		Skipped:  make([]skippedLesson, 0),
	}
	if len(lessons) == 0 {
		resp.Message = "Nenhuma lesson pendente encontrada para este tenant"
		return resp, http.StatusOK, nil
	}

	for _, l := range lessons {
		payload, err := json.Marshal(jobPayload{
			LessonID: l.ID,
			TenantID: tenantID,
			VideoURL: l.MediaURL,
			CourseID: l.CourseID,
			Title:    l.Name,
		})
		if err != nil {
			resp.Skipped = append(resp.Skipped, skippedLesson{LessonID: l.ID, Reason: "erro interno ao serializar payload"})
			continue
		}
		jobID := uuid.NewString()
		if _, err := f.transcriptionDB.ExecContext(ctx, sqlInsertJob,
			jobID, tenantID, 0, payload, 3,
		); err != nil {
			f.log.Error("transcription.enqueue.insert_failed",
				"tenant", tenantID, "lesson", l.ID, "error", err.Error())
			resp.Skipped = append(resp.Skipped, skippedLesson{LessonID: l.ID, Reason: "erro ao gravar job"})
			continue
		}
		resp.Enqueued = append(resp.Enqueued, enqueuedLesson{LessonID: l.ID, JobID: jobID})
	}

	resp.EnqueuedCount = len(resp.Enqueued)
	resp.Success = resp.EnqueuedCount > 0
	if resp.Success {
		resp.Message = fmt.Sprintf("%d job(s) de transcrição enfileirado(s) (todas as pendentes)", resp.EnqueuedCount)
	} else {
		resp.Message = "nenhuma lesson elegível foi enfileirada"
	}
	return resp, http.StatusAccepted, nil
}

func (f *Feature) enqueueSelectedLessons(ctx context.Context, tenantID string, lessonIDs []string) (*processLessonsResponse, int, error) {
	// Validate the tenant has aiEnabled. The same query the pipeline
	// runs is reused so error codes stay consistent.
	var (
		tID, tName              string
		aiEnabled               bool
		bunnyLibID, bunnyAPIKey *string
	)
	row := f.memberclassDB.QueryRowContext(ctx, sqlSelectTenantBunnyCreds, tenantID)
	if err := row.Scan(&tID, &tName, &aiEnabled, &bunnyLibID, &bunnyAPIKey); err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("tenant não encontrado")
	}
	if !aiEnabled {
		return nil, http.StatusForbidden, fmt.Errorf("IA não está habilitada para este tenant")
	}

	// Pull lessons that match the id set AND belong to this tenant
	// (the Vitrine join enforces ownership).
	rows, err := f.memberclassDB.QueryContext(ctx, sqlSelectLessonsByIDs, tenantID, pq.Array(lessonIDs))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("listar lessons: %w", err)
	}
	defer rows.Close()

	type lessonRow struct {
		ID                     string
		Name                   string
		MediaURL               string
		CourseID               string
		TranscriptionCompleted bool
	}
	found := make(map[string]lessonRow, len(lessonIDs))
	for rows.Next() {
		var (
			l                                                                                  lessonRow
			slug                                                                               string
			lessonType, thumbnail, content                                                     *string
			moduleID, moduleName, sectionID, sectionName, courseName, vitrineID, vitrineName   string
		)
		if err := rows.Scan(
			&l.ID, &l.Name, &slug,
			&lessonType, &l.MediaURL, &thumbnail, &content,
			&moduleID, &moduleName, &sectionID, &sectionName,
			&l.CourseID, &courseName, &vitrineID, &vitrineName,
			&l.TranscriptionCompleted,
		); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("scan lesson: %w", err)
		}
		found[l.ID] = l
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("iterate lessons: %w", err)
	}

	resp := &processLessonsResponse{
		TenantID: tenantID,
		Enqueued: make([]enqueuedLesson, 0, len(lessonIDs)),
		Skipped:  make([]skippedLesson, 0),
	}

	for _, id := range lessonIDs {
		l, ok := found[id]
		if !ok {
			resp.Skipped = append(resp.Skipped, skippedLesson{
				LessonID: id,
				Reason:   "lesson não encontrada para este tenant ou mediaUrl não é Bunny",
			})
			continue
		}
		if l.TranscriptionCompleted {
			resp.Skipped = append(resp.Skipped, skippedLesson{
				LessonID: id,
				Reason:   "já transcrita (transcriptionCompleted=true)",
			})
			continue
		}

		payload, err := json.Marshal(jobPayload{
			LessonID: l.ID,
			TenantID: tenantID,
			VideoURL: l.MediaURL,
			CourseID: l.CourseID,
			Title:    l.Name,
		})
		if err != nil {
			f.log.Error("transcription.enqueue.marshal_failed", "lesson", l.ID, "error", err.Error())
			resp.Skipped = append(resp.Skipped, skippedLesson{LessonID: id, Reason: "erro interno ao serializar payload"})
			continue
		}
		jobID := uuid.NewString()
		if _, err := f.transcriptionDB.ExecContext(ctx, sqlInsertJob,
			jobID, tenantID, 0, payload, 3,
		); err != nil {
			f.log.Error("transcription.enqueue.insert_failed",
				"tenant", tenantID, "lesson", l.ID, "error", err.Error())
			resp.Skipped = append(resp.Skipped, skippedLesson{LessonID: id, Reason: "erro ao gravar job"})
			continue
		}
		resp.Enqueued = append(resp.Enqueued, enqueuedLesson{LessonID: id, JobID: jobID})
	}

	resp.EnqueuedCount = len(resp.Enqueued)
	if resp.EnqueuedCount == 0 {
		resp.Success = false
		resp.Message = "nenhuma lesson elegível foi enfileirada"
	} else {
		resp.Success = true
		resp.Message = fmt.Sprintf("%d job(s) de transcrição enfileirado(s)", resp.EnqueuedCount)
	}
	return resp, http.StatusAccepted, nil
}
