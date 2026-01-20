package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/auth"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	auth2 "github.com/memberclass-backend-golang/internal/domain/ports/auth"
)

type AuthHandler struct {
	useCase auth2.AuthUseCase
	logger  ports.Logger
}

func NewAuthHandler(useCase auth2.AuthUseCase, logger ports.Logger) *AuthHandler {
	return &AuthHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *AuthHandler) GenerateMagicLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	var req auth.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Email é obrigatório e deve ser uma string", "INVALID_REQUEST")
		return
	}

	response, err := h.useCase.GenerateMagicLink(r.Context(), req, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *AuthHandler) handleUseCaseError(w http.ResponseWriter, err error) {
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
		if memberClassErr.Message == "Usuário não encontrado" {
			h.sendCustomErrorResponse(w, http.StatusNotFound, memberClassErr.Message, "USER_NOT_FOUND")
		} else {
			h.sendCustomErrorResponse(w, http.StatusNotFound, memberClassErr.Message, "USER_NOT_IN_TENANT")
		}
	case 429:
		h.sendCustomErrorResponse(w, http.StatusTooManyRequests, memberClassErr.Message, "RATE_LIMIT_EXCEEDED")
	default:
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
	}
}

func (h *AuthHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AuthHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *AuthHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
