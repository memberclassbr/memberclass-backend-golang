package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/comments"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/comment"
	comment2 "github.com/memberclass-backend-golang/internal/domain/usecases/comment"
)

type CommentHandler struct {
	useCase comment.CommentUseCase
	logger  ports.Logger
}

func NewCommentHandler(useCase comment.CommentUseCase, logger ports.Logger) *CommentHandler {
	return &CommentHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *CommentHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendCustomErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	parsedReq, err := comments.ParseGetCommentsRequest(r.URL.Query())
	if err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100", "INVALID_PAGINATION")
		return
	}

	if err := parsedReq.Validate(); err != nil {
		if err.Error() == "page deve ser >= 1" || err.Error() == "limit deve ser entre 1 e 100" {
			h.sendCustomErrorResponse(w, http.StatusBadRequest, "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100", "INVALID_PAGINATION")
			return
		}
		h.sendCustomErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	response, err := h.useCase.GetComments(r.Context(), tenant.ID, parsedReq)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.sendCustomErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	commentID := chi.URLParam(r, "commentID")
	if commentID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Comment ID is required", "INVALID_REQUEST")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	var req comments.UpdateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_REQUEST")
		return
	}

	response, err := h.useCase.UpdateAnswer(r.Context(), commentID, tenant.ID, req)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	// Formato de resposta: {ok: true, comment: {...}}
	result := map[string]interface{}{
		"ok":      true,
		"comment": response,
	}

	h.sendJSONResponse(w, http.StatusOK, result)
}

func (h *CommentHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		errorCode := "INTERNAL_ERROR"
		if memberClassErr.Code == 404 {
			if memberClassErr.Message == "Usuário não encontrado" {
				errorCode = "USER_NOT_FOUND"
			} else if memberClassErr.Message == "Usuário não está associado a este tenant" {
				errorCode = "USER_NOT_IN_TENANT"
			} else {
				errorCode = "COMMENT_NOT_FOUND"
			}
		} else if memberClassErr.Code == 400 {
			errorCode = "INVALID_REQUEST"
		}
		h.sendCustomErrorResponse(w, memberClassErr.Code, memberClassErr.Message, errorCode)
		return
	}

	if errors.Is(err, comment2.ErrCommentNotFound) {
		h.sendCustomErrorResponse(w, http.StatusNotFound, "Comentário não encontrado ou não pertence a este tenant", "COMMENT_NOT_FOUND")
		return
	}

	if errors.Is(err, comment2.ErrAnswerRequired) {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Campo 'answer' é obrigatório e deve ser uma string", "INVALID_REQUEST")
		return
	}

	h.logger.Error("Unexpected error: " + err.Error())
	h.sendCustomErrorResponse(w, http.StatusInternalServerError, "Erro interno do servidor", "INTERNAL_ERROR")
}

func (h *CommentHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := dto.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *CommentHandler) sendCustomErrorResponse(w http.ResponseWriter, code int, message, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]interface{}{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *CommentHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
