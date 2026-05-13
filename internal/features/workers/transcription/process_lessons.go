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
// transcription for a hand-picked set of lessons. We deliberately do
// NOT support a "transcribe everything unprocessed" mode here — that
// used to be a daily cron and burned OpenAI credit on lessons no one
// asked for. The operator must select the rows in the admin and the
// frontend ships the explicit id list.
type processLessonsRequest struct {
	TenantID  string   `json:"tenantId"`
	LessonIDs []string `json:"lessonIds"`
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
	if len(req.LessonIDs) == 0 {
		writeCustomError(w, http.StatusBadRequest,
			"lessonIds é obrigatório (selecione ao menos uma aula no administrativo)",
			"MISSING_LESSON_IDS")
		return
	}

	resp, status, err := f.enqueueSelectedLessons(r.Context(), req.TenantID, req.LessonIDs)
	if err != nil {
		writeError(w, status, http.StatusText(status), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// ---------- 2. Business rule ----------

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
