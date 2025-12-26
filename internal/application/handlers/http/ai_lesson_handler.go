package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type AILessonHandler struct {
	useCase ports.AILessonUseCase
	logger  ports.Logger
}

func NewAILessonHandler(useCase ports.AILessonUseCase, logger ports.Logger) *AILessonHandler {
	return &AILessonHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *AILessonHandler) UpdateTranscriptionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	apiKey := r.Header.Get("x-internal-api-key")
	expectedKey := os.Getenv("INTERNAL_AI_API_KEY")
	if apiKey == "" || apiKey != expectedKey {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "lessonId é obrigatório", "INVALID_REQUEST")
		return
	}

	var req request.UpdateLessonTranscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "transcriptionCompleted deve ser um booleano", "INVALID_REQUEST")
		return
	}

	response, err := h.useCase.UpdateTranscriptionStatus(r.Context(), lessonID, req)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *AILessonHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	memberClassErr, ok := err.(*memberclasserrors.MemberClassError)
	if !ok {
		var asErr *memberclasserrors.MemberClassError
		if errors.As(err, &asErr) {
			memberClassErr = asErr
		} else {
			h.logger.Error("Unexpected error: " + err.Error())
			h.sendErrorResponse(w, http.StatusInternalServerError, "Erro interno do servidor")
			return
		}
	}

	if memberClassErr == nil {
		h.logger.Error("Unexpected error: " + err.Error())
		h.sendErrorResponse(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	switch memberClassErr.Code {
	case 400:
		h.sendCustomErrorResponse(w, http.StatusBadRequest, memberClassErr.Message, "INVALID_REQUEST")
	case 404:
		h.sendCustomErrorResponse(w, http.StatusNotFound, memberClassErr.Message, "LESSON_NOT_FOUND")
	case 403:
		h.sendCustomErrorResponse(w, http.StatusForbidden, memberClassErr.Message, "AI_NOT_ENABLED")
	case 429:
		h.sendCustomErrorResponse(w, http.StatusTooManyRequests, memberClassErr.Message, "RATE_LIMIT_EXCEEDED")
	default:
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
	}
}

func (h *AILessonHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AILessonHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AILessonHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

