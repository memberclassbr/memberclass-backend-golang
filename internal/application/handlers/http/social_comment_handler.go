package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
)

type SocialCommentHandler struct {
	useCase ports.SocialCommentUseCase
	logger  ports.Logger
}

func NewSocialCommentHandler(useCase ports.SocialCommentUseCase, logger ports.Logger) *SocialCommentHandler {
	return &SocialCommentHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *SocialCommentHandler) CreateOrUpdatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req request.CreateSocialCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key invalid", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.CreateOrUpdatePost(r.Context(), req, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *SocialCommentHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	if errors.Is(err, usecases.ErrUserNotFoundOrNotInTenantForPost) {
		h.sendCustomErrorResponse(w, http.StatusForbidden, "Usuário não pertence ao tenant", "PERMISSION_DENIED")
		return
	}

	if errors.Is(err, usecases.ErrPostNotFound) {
		h.sendCustomErrorResponse(w, http.StatusNotFound, "Post não encontrado", "POST_NOT_FOUND")
		return
	}

	if errors.Is(err, usecases.ErrTopicNotFound) {
		h.sendCustomErrorResponse(w, http.StatusNotFound, "Tópico não existe", "TOPIC_NOT_FOUND")
		return
	}

	if errors.Is(err, usecases.ErrPermissionDenied) {
		h.sendCustomErrorResponse(w, http.StatusForbidden, "Você não tem autorização para fazer esta ação", "PERMISSION_DENIED")
		return
	}

	if errors.Is(err, usecases.ErrNoAccessToTopic) {
		h.sendCustomErrorResponse(w, http.StatusForbidden, "Você não tem acesso para publicar neste tópico", "NO_ACCESS_TO_TOPIC")
		return
	}

	errMsg := err.Error()
	if errMsg == "userId é obrigatório" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "userId é obrigatório", "MISSING_USER")
		return
	}
	if errMsg == "topicId é obrigatório para criar post" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "topicId é obrigatório para criar post", "MISSING_TOPIC")
		return
	}
	if errMsg == "title é obrigatório" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "title é obrigatório", "MISSING_TITLE")
		return
	}
	if errMsg == "content é obrigatório" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "content é obrigatório", "MISSING_CONTENT")
		return
	}

	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
		return
	}

	h.logger.Error("Unexpected error: " + err.Error())
	h.sendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
}

func (h *SocialCommentHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *SocialCommentHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := dto.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *SocialCommentHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

