package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/lib/pq"
)

// ---------- DTOs ----------

// searchRequest is the body of POST /api/v1/ai/search. `tenantId` and
// `query` are required; the scope filters are all optional and combine
// from narrowest (lessonId) to broadest (tenantId only). When more than
// one is supplied the narrowest non-empty one wins — the others are
// ignored — so the admin UI can ship the full hierarchy without having
// to clear sibling fields.
type searchRequest struct {
	TenantID string      `json:"tenantId"`
	Query    string      `json:"query"`
	Scope    searchScope `json:"scope"`
	Limit    int         `json:"limit"`
}

type searchScope struct {
	CourseID  string `json:"courseId,omitempty"`
	SectionID string `json:"sectionId,omitempty"`
	ModuleID  string `json:"moduleId,omitempty"`
	LessonID  string `json:"lessonId,omitempty"`
}

// searchHit is one chunk returned to the caller. Similarity is the
// cosine similarity (1 - distance), so 1.0 is a perfect match and 0.0
// is orthogonal. start_time / end_time are seconds into the source
// video so the frontend can deep-link.
type searchHit struct {
	ChunkID    string  `json:"chunkId"`
	LessonID   string  `json:"lessonId"`
	CourseID   string  `json:"courseId,omitempty"`
	VideoID    string  `json:"videoId"`
	Text       string  `json:"text"`
	StartTime  float64 `json:"startTime"`
	EndTime    float64 `json:"endTime"`
	Similarity float64 `json:"similarity"`
}

type searchResponse struct {
	TenantID string      `json:"tenantId"`
	Query    string      `json:"query"`
	Scope    searchScope `json:"scope"`
	Hits     []searchHit `json:"hits"`
	Count    int         `json:"count"`
}

const (
	searchDefaultLimit = 10
	searchMaxLimit     = 20
)

// ---------- 1. HTTP handler ----------

// Search handles `POST /api/v1/ai/search`. Returns up to `limit` chunks
// (default 10, max 20) ranked by cosine similarity to the user's query
// embedding. Scope narrowest-wins:
//
//	lessonId  → chunks.lesson_id = lessonId
//	moduleId  → memberclass lookup → chunks.lesson_id IN (...)
//	sectionId → memberclass lookup → chunks.lesson_id IN (...)
//	courseId  → chunks.course_id = courseId
//	(none)    → chunks.tenant_id = tenantId
//
// The handler reuses the slice's embedBatch helper to embed the query,
// then runs cosine similarity (`<=>` operator) against the HNSW index
// on chunks.embedding.
func (f *Feature) Search(w http.ResponseWriter, r *http.Request) {
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

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCustomError(w, http.StatusBadRequest, "JSON inválido", "INVALID_REQUEST")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.TenantID == "" {
		writeCustomError(w, http.StatusBadRequest, "tenantId é obrigatório", "MISSING_TENANT_ID")
		return
	}
	if req.Query == "" {
		writeCustomError(w, http.StatusBadRequest, "query é obrigatório", "MISSING_QUERY")
		return
	}
	if req.Limit <= 0 {
		req.Limit = searchDefaultLimit
	}
	if req.Limit > searchMaxLimit {
		req.Limit = searchMaxLimit
	}

	resp, status, err := f.searchChunks(r.Context(), req)
	if err != nil {
		writeError(w, status, http.StatusText(status), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) searchChunks(ctx context.Context, req searchRequest) (*searchResponse, int, error) {
	// 1. Embed the query (single input batch).
	vecs, _, err := f.embedBatch(ctx, []string{req.Query})
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, http.StatusInternalServerError, fmt.Errorf("embed query: got %d vectors, want 1", len(vecs))
	}
	queryVec := pgvectorString(vecs[0])

	// 2. Resolve scope into a lesson_id allow-list when needed.
	lessonIDs, err := f.resolveScopeLessonIDs(ctx, req.Scope)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	// 3. Build the SQL — scope dictates the WHERE clause and the
	//    parameter layout. Embedding always rides $1; tenant always $2.
	sqlText, args := buildSearchQuery(queryVec, req.TenantID, req.Scope, lessonIDs, req.Limit)

	rows, err := f.transcriptionDB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	hits := make([]searchHit, 0, req.Limit)
	for rows.Next() {
		var (
			h        searchHit
			courseID *string
		)
		if err := rows.Scan(
			&h.ChunkID, &h.LessonID, &courseID, &h.VideoID,
			&h.Text, &h.StartTime, &h.EndTime, &h.Similarity,
		); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("scan hit: %w", err)
		}
		if courseID != nil {
			h.CourseID = *courseID
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("iterate hits: %w", err)
	}

	return &searchResponse{
		TenantID: req.TenantID,
		Query:    req.Query,
		Scope:    req.Scope,
		Hits:     hits,
		Count:    len(hits),
	}, http.StatusOK, nil
}

// resolveScopeLessonIDs returns the explicit lesson_id list when the
// scope is moduleId or sectionId (chunks table doesn't carry those
// columns — we have to go to memberclass). Returns nil + nil error for
// scopes that don't need a lookup; callers must inspect req.Scope to
// know whether the slice means "no filter" or "filter is direct on
// chunks.lesson_id".
func (f *Feature) resolveScopeLessonIDs(ctx context.Context, scope searchScope) ([]string, error) {
	switch {
	case scope.LessonID != "":
		return nil, nil // direct chunks.lesson_id = $X path
	case scope.ModuleID != "":
		return f.fetchLessonIDs(ctx, sqlLessonsByModule, scope.ModuleID)
	case scope.SectionID != "":
		return f.fetchLessonIDs(ctx, sqlLessonsBySection, scope.SectionID)
	default:
		return nil, nil
	}
}

func (f *Feature) fetchLessonIDs(ctx context.Context, query, arg string) ([]string, error) {
	rows, err := f.memberclassDB.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("resolve scope: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan scope lesson id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("scope sem aulas correspondentes")
	}
	return ids, nil
}

// ---------- 3. SQL ----------

// buildSearchQuery composes the search SQL with the right WHERE clause
// for the resolved scope. $1 is always the embedding vector literal,
// $2 is always the tenant id, additional params depend on the scope.
// `<=>` is pgvector's cosine distance operator; we expose `1 - distance`
// as the similarity score so 1.0 means identical and 0.0 means
// orthogonal.
func buildSearchQuery(queryVec, tenantID string, scope searchScope, lessonIDs []string, limit int) (string, []any) {
	const base = `
        SELECT id, lesson_id, course_id, video_id, text, start_time, end_time,
               1 - (embedding <=> $1::vector) AS similarity
          FROM chunks
         WHERE tenant_id = $2 AND embedding IS NOT NULL
    `

	args := []any{queryVec, tenantID}
	var where string

	switch {
	case scope.LessonID != "":
		args = append(args, scope.LessonID)
		where = fmt.Sprintf(" AND lesson_id = $%d", len(args))
	case len(lessonIDs) > 0:
		args = append(args, pq.Array(lessonIDs))
		where = fmt.Sprintf(" AND lesson_id = ANY($%d)", len(args))
	case scope.CourseID != "":
		args = append(args, scope.CourseID)
		where = fmt.Sprintf(" AND course_id = $%d", len(args))
	}

	args = append(args, limit)
	tail := fmt.Sprintf(`
         ORDER BY embedding <=> $1::vector ASC
         LIMIT $%d
    `, len(args))

	return base + where + tail, args
}
