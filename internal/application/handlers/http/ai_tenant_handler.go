package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type AITenantHandler struct {
	useCase ports.AITenantUseCase
	logger  ports.Logger
}

func NewAITenantHandler(useCase ports.AITenantUseCase, logger ports.Logger) *AITenantHandler {
	return &AITenantHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *AITenantHandler) GetTenantsWithAIEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	apiKey := r.Header.Get("x-internal-api-key")
	expectedKey := os.Getenv("INTERNAL_AI_API_KEY")
	if apiKey == "" || apiKey != expectedKey {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	response, err := h.useCase.GetTenantsWithAIEnabled(r.Context())
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *AITenantHandler) ProcessLessonsTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	apiKey := r.Header.Get("x-internal-api-key")
	expectedKey := os.Getenv("INTERNAL_AI_API_KEY")
	if apiKey == "" || apiKey != expectedKey {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	var req request.ProcessLessonsTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "Corpo da requisição inválido")
		return
	}

	response, err := h.useCase.ProcessLessonsTenant(r.Context(), req)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	statusCode := http.StatusOK
	if response.Success {
		statusCode = http.StatusAccepted
	}

	h.sendJSONResponse(w, statusCode, response)
}

func (h *AITenantHandler) handleUseCaseError(w http.ResponseWriter, err error) {
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
	case 429:
		h.sendCustomErrorResponse(w, http.StatusTooManyRequests, memberClassErr.Message, "RATE_LIMIT_EXCEEDED")
	default:
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
	}
}

func (h *AITenantHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AITenantHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AITenantHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
