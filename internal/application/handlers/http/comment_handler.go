package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
)

type CommentHandler struct {
	useCase         ports.CommentUseCase
	logger          ports.Logger
	paginationUtils *usecases.PaginationUtils
}

func NewCommentHandler(useCase ports.CommentUseCase, logger ports.Logger) *CommentHandler {
	return &CommentHandler{
		useCase:         useCase,
		logger:          logger,
		paginationUtils: usecases.NewPaginationUtils(),
	}
}

func (h *CommentHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendErrorResponse(w, http.StatusUnauthorized, "Tenant not found in context")
		return
	}

	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	pagination := h.paginationUtils.ParsePaginationFromQuery(queryParams)

	response, err := h.useCase.GetComments(r.Context(), tenant.ID, pagination)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	commentID := chi.URLParam(r, "commentID")
	if commentID == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "Comment ID is required")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendErrorResponse(w, http.StatusUnauthorized, "Tenant not found in context")
		return
	}

	var req request.UpdateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	response, err := h.useCase.UpdateAnswer(r.Context(), commentID, tenant.ID, req)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *CommentHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
		return
	}

	if errors.Is(err, usecases.ErrCommentNotFound) {
		h.sendErrorResponse(w, http.StatusNotFound, "Comment not found")
		return
	}

	if errors.Is(err, usecases.ErrAnswerRequired) {
		h.sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	h.logger.Error("Unexpected error: " + err.Error())
	h.sendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
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

func (h *CommentHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
