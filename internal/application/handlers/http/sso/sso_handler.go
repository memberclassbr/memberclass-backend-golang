package sso

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/sso"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	ssoports "github.com/memberclass-backend-golang/internal/domain/ports/sso"
)

type SSOHandler struct {
	useCase ssoports.SSOUseCase
	logger  ports.Logger
}

func NewSSOHandler(useCase ssoports.SSOUseCase, logger ports.Logger) *SSOHandler {
	return &SSOHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *SSOHandler) GenerateSSOToken(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("x-internal-api-key")
	expectedKey := os.Getenv("INTERNAL_AI_API_KEY")
	if apiKey == "" || apiKey != expectedKey {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req sso.GenerateSSOTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Requisição inválida", "INVALID_REQUEST")
		return
	}

	if req.UserID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "userId é obrigatório", "INVALID_REQUEST")
		return
	}

	if req.TenantID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "tenantId é obrigatório", "INVALID_REQUEST")
		return
	}

	externalURL := r.URL.Query().Get("externalUrl")
	if externalURL == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "externalUrl é obrigatório", "INVALID_REQUEST")
		return
	}

	response, err := h.useCase.GenerateSSOToken(r.Context(), req, externalURL)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *SSOHandler) ValidateSSOToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req sso.ValidateSSOTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Requisição inválida", "INVALID_REQUEST")
		return
	}

	if req.Token == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "token é obrigatório", "INVALID_REQUEST")
		return
	}

	ip := h.getClientIP(r)

	response, err := h.useCase.ValidateSSOToken(r.Context(), req.Token, ip)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *SSOHandler) getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

func (h *SSOHandler) handleUseCaseError(w http.ResponseWriter, err error) {
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
	case 401:
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, memberClassErr.Message, "INVALID_TOKEN")
	case 403:
		h.sendCustomErrorResponse(w, http.StatusForbidden, memberClassErr.Message, "FORBIDDEN")
	case 404:
		h.sendCustomErrorResponse(w, http.StatusNotFound, memberClassErr.Message, "NOT_FOUND")
	default:
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
	}
}

func (h *SSOHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *SSOHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *SSOHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
