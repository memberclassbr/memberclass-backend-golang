package comment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// ---------- DTOs ----------

type updateCommentRequest struct {
	Answer string `json:"answer"`
	// Published is a pointer so an absent field is distinguishable from
	// `false`; absent means "not published".
	Published *bool `json:"published"`
}

// errCommentNotFound covers both a comment that does not exist and one that
// belongs to another tenant — the two are deliberately indistinguishable.
var errCommentNotFound = errors.New("comentário não encontrado ou não pertence a este tenant")

// ---------- 1. HTTP handler ----------

// UpdateComment handles `PATCH /api/v1/comments/{commentID}`: writes the
// instructor's answer and the moderation flag, then returns the comment with
// its lesson, course and author resolved.
func (f *Feature) UpdateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	commentID := chi.URLParam(r, "commentID")
	if commentID == "" {
		writeError(w, http.StatusBadRequest, "Comment ID is required", "INVALID_REQUEST")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	var req updateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", "INVALID_REQUEST")
		return
	}

	updated, err := f.updateAnswer(r.Context(), commentID, tenant.ID, req)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"comment": updated,
	})
}

// ---------- 2. Business rule ----------

func (f *Feature) updateAnswer(ctx context.Context, commentID, tenantID string, req updateCommentRequest) (*commentResponse, error) {
	if req.Answer == "" {
		return nil, errAnswerRequired
	}

	// Confirm the comment belongs to this tenant before writing: the UPDATE
	// itself is keyed only by comment id.
	if err := f.commentBelongsToTenant(ctx, commentID, tenantID); err != nil {
		return nil, err
	}

	published := req.Published != nil && *req.Published

	if err := f.applyAnswer(ctx, commentID, req.Answer, published); err != nil {
		return nil, err
	}

	return f.commentWithDetails(ctx, commentID, tenantID)
}

var errAnswerRequired = errors.New("campo 'answer' é obrigatório e deve ser uma string")

// ---------- 3. SQL ----------

const (
	sqlCommentExists = `
        SELECT c.id` + sqlCommentsFrom + `
        WHERE c.id = $1 AND v."tenantId" = $2
        LIMIT 1
    `

	sqlUpdateComment = `
        UPDATE "Comment"
        SET answer = $2, published = $3, "updatedAt" = $4
        WHERE id = $1
    `

	sqlCommentWithDetails = sqlCommentsSelect + ` AND c.id = $2 LIMIT 1`
)

func (f *Feature) commentBelongsToTenant(ctx context.Context, commentID, tenantID string) error {
	var id string
	err := f.db.QueryRowContext(ctx, sqlCommentExists, commentID, tenantID).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errCommentNotFound
	case err != nil:
		return f.fail("Error finding comment: ", err, "error finding comment")
	}
	return nil
}

func (f *Feature) applyAnswer(ctx context.Context, commentID, answer string, published bool) error {
	_, err := f.db.ExecContext(ctx, sqlUpdateComment, commentID, answer, published, time.Now())
	if err != nil {
		return f.fail("Error updating comment: ", err, "error updating comment")
	}
	return nil
}

func (f *Feature) commentWithDetails(ctx context.Context, commentID, tenantID string) (*commentResponse, error) {
	row := f.db.QueryRowContext(ctx, sqlCommentWithDetails, tenantID, commentID)

	c, err := scanComment(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, errCommentNotFound
	case err != nil:
		return nil, f.fail("Error reading updated comment: ", err, "error reading updated comment")
	}
	return &c, nil
}

// ---------- error mapping ----------

// writeUseCaseError maps a rule failure to the status and error code clients
// switch on.
func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errCommentNotFound):
		writeError(w, http.StatusNotFound, "Comentário não encontrado ou não pertence a este tenant", "COMMENT_NOT_FOUND")
		return
	case errors.Is(err, errAnswerRequired):
		writeError(w, http.StatusBadRequest, "Campo 'answer' é obrigatório e deve ser uma string", "INVALID_REQUEST")
		return
	}

	var mcErr *memberclasserrors.MemberClassError
	if errors.As(err, &mcErr) {
		writeError(w, mcErr.Code, mcErr.Message, errorCodeFor(mcErr))
		return
	}

	f.log.Error("Unexpected error: " + err.Error())
	writeError(w, http.StatusInternalServerError, "Erro interno do servidor", "INTERNAL_ERROR")
}

func errorCodeFor(err *memberclasserrors.MemberClassError) string {
	switch err.Code {
	case http.StatusNotFound:
		switch err.Message {
		case "Usuário não encontrado":
			return "USER_NOT_FOUND"
		case "Usuário não está associado a este tenant":
			return "USER_NOT_IN_TENANT"
		default:
			return "COMMENT_NOT_FOUND"
		}
	case http.StatusBadRequest:
		return "INVALID_REQUEST"
	default:
		return "INTERNAL_ERROR"
	}
}

// ---------- responses ----------

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
