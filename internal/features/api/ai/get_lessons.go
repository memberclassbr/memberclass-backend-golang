package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

// ---------- DTOs ----------

type aiLessonData struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	Slug                   string  `json:"slug"`
	Type                   *string `json:"type"`
	MediaURL               *string `json:"mediaUrl"`
	Thumbnail              *string `json:"thumbnail"`
	Content                *string `json:"content"`
	TranscriptionCompleted bool    `json:"transcriptionCompleted"`
	ModuleID               string  `json:"moduleId"`
	ModuleName             string  `json:"moduleName"`
	SectionID              string  `json:"sectionId"`
	SectionName            string  `json:"sectionName"`
	CourseID               string  `json:"courseId"`
	CourseName             string  `json:"courseName"`
	VitrineID              string  `json:"vitrineId"`
	VitrineName            string  `json:"vitrineName"`
}

type aiLessonsResponse struct {
	Lessons         []aiLessonData `json:"lessons"`
	Total           int            `json:"total"`
	TenantID        string         `json:"tenantId"`
	OnlyUnprocessed bool           `json:"onlyUnprocessed"`
}

// ---------- 1. HTTP handler ----------

// GetLessons handles `GET /api/v1/ai/lessons?tenantId=&onlyUnprocessed=`.
func (f *Feature) GetLessons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !f.authorized(w, r) {
		return
	}

	tenantID := r.URL.Query().Get("tenantId")
	// Anything other than the exact string "true" means false, which is how
	// this flag has always been read.
	onlyUnprocessed := r.URL.Query().Get("onlyUnprocessed") == "true"

	resp, err := f.getLessons(r.Context(), tenantID, onlyUnprocessed)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) getLessons(ctx context.Context, tenantID string, onlyUnprocessed bool) (*aiLessonsResponse, error) {
	if tenantID == "" {
		return nil, &memberclasserrors.MemberClassError{Code: 400, Message: "tenantId é obrigatório"}
	}

	// The AI dashboard may only list lessons of a tenant that has the feature
	// switched on.
	aiEnabled, err := f.tenantAIEnabled(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !aiEnabled {
		return nil, &memberclasserrors.MemberClassError{Code: 403, Message: "IA não está habilitada para este tenant"}
	}

	lessons, err := f.queryLessons(ctx, tenantID, onlyUnprocessed)
	if err != nil {
		return nil, err
	}

	return &aiLessonsResponse{
		Lessons:         lessons,
		Total:           len(lessons),
		TenantID:        tenantID,
		OnlyUnprocessed: onlyUnprocessed,
	}, nil
}

// ---------- 3. SQL ----------

const (
	sqlTenantAIEnabled = `SELECT "aiEnabled" FROM "Tenant" WHERE id = $1`

	// sqlAILessons walks each lesson up to its vitrine so the dashboard can
	// show the full breadcrumb, and orders by the same "order" column at every
	// level so the listing matches the catalog's ordering.
	//
	// The mediaUrl filter restricts the listing to Bunny-hosted videos, the
	// only ones the transcription pipeline could originally handle.
	sqlAILessons = `
		SELECT
			l.id,
			l.name,
			l.slug,
			l.type,
			l."mediaUrl",
			l.thumbnail,
			l.content,
			l."transcriptionCompleted",
			m.id as module_id,
			m.name as module_name,
			s.id as section_id,
			s.name as section_name,
			c.id as course_id,
			c.name as course_name,
			v.id as vitrine_id,
			v.name as vitrine_name
		FROM "Lesson" l
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE v."tenantId" = $1
			AND l.published = true
			AND ($2 = false OR l."transcriptionCompleted" = false)
			AND l."mediaUrl" LIKE '%https://iframe.mediadelivery.net%'
		ORDER BY
			COALESCE(v."order", 0) ASC,
			COALESCE(c."order", 0) ASC,
			COALESCE(s."order", 0) ASC,
			COALESCE(m."order", 0) ASC,
			COALESCE(l."order", 0) ASC
	`
)

func (f *Feature) tenantAIEnabled(ctx context.Context, tenantID string) (bool, error) {
	var enabled bool
	err := f.db.QueryRowContext(ctx, sqlTenantAIEnabled, tenantID).Scan(&enabled)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, &memberclasserrors.MemberClassError{Code: 404, Message: "Tenant não encontrado"}
	case err != nil:
		return false, f.fail("Error finding tenant: ", err, "error finding tenant")
	}
	return enabled, nil
}

func (f *Feature) queryLessons(ctx context.Context, tenantID string, onlyUnprocessed bool) ([]aiLessonData, error) {
	rows, err := f.db.QueryContext(ctx, sqlAILessons, tenantID, onlyUnprocessed)
	if err != nil {
		return nil, f.fail("Error querying lessons with hierarchy: ", err, "error querying lessons with hierarchy")
	}
	defer rows.Close()

	lessons := make([]aiLessonData, 0)
	for rows.Next() {
		var l aiLessonData
		var lessonType, mediaURL, thumbnail, content sql.NullString
		var transcriptionCompleted sql.NullBool

		err := rows.Scan(
			&l.ID, &l.Name, &l.Slug,
			&lessonType, &mediaURL, &thumbnail, &content,
			&transcriptionCompleted,
			&l.ModuleID, &l.ModuleName,
			&l.SectionID, &l.SectionName,
			&l.CourseID, &l.CourseName,
			&l.VitrineID, &l.VitrineName,
		)
		if err != nil {
			return nil, f.fail("Error scanning lesson with hierarchy: ", err, "error scanning lesson with hierarchy")
		}

		l.Type = strPtr(lessonType)
		l.MediaURL = strPtr(mediaURL)
		l.Thumbnail = strPtr(thumbnail)
		l.Content = strPtr(content)
		l.TranscriptionCompleted = transcriptionCompleted.Valid && transcriptionCompleted.Bool

		lessons = append(lessons, l)
	}

	if err := rows.Err(); err != nil {
		return nil, f.fail("Error iterating lessons with hierarchy: ", err, "error iterating lessons with hierarchy")
	}

	return lessons, nil
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}

// ---------- errors and responses ----------

func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

// writeUseCaseError maps a rule failure to the status and error code the AI
// dashboard switches on.
func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if !errors.As(err, &mcErr) || mcErr == nil {
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	switch mcErr.Code {
	case http.StatusBadRequest:
		writeCustomError(w, http.StatusBadRequest, mcErr.Message, "INVALID_REQUEST")
	case http.StatusNotFound:
		writeCustomError(w, http.StatusNotFound, mcErr.Message, "LESSON_NOT_FOUND")
	case http.StatusForbidden:
		writeCustomError(w, http.StatusForbidden, mcErr.Message, "AI_NOT_ENABLED")
	case http.StatusTooManyRequests:
		writeCustomError(w, http.StatusTooManyRequests, mcErr.Message, "RATE_LIMIT_EXCEEDED")
	default:
		writeError(w, mcErr.Code, mcErr.Message)
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError uses the {error, message} shape, for 405 and unmapped codes.
func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{
		"error":   http.StatusText(code),
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
