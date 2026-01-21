package lesson

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/ports/lesson"
	"github.com/memberclass-backend-golang/internal/domain/usecases/lessons"
)

type LessonsCompletedHandler struct {
	useCase lesson2.LessonsCompletedUseCase
	logger  ports.Logger
}

func NewLessonsCompletedHandler(useCase lesson2.LessonsCompletedUseCase, logger ports.Logger) *LessonsCompletedHandler {
	return &LessonsCompletedHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *LessonsCompletedHandler) GetLessonsCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key invalid", "INVALID_API_KEY")
		return
	}

	req, err := lesson.ParseLessonsCompletedRequest(r.URL.Query())
	if err != nil {
		h.handleParseError(w, err)
		return
	}

	if err := req.Validate(); err != nil {
		h.handleValidationError(w, err)
		return
	}

	response, err := h.useCase.GetLessonsCompleted(r.Context(), *req, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *LessonsCompletedHandler) handleParseError(w http.ResponseWriter, err error) {
	errMsg := err.Error()
	if errMsg == "page deve ser um número" || errMsg == "limit deve ser um número" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_PAGINATION")
		return
	}
	if errMsg == "formato de data inválido para startDate" || errMsg == "formato de data inválido para endDate" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_DATE_FORMAT")
		return
	}
	h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_REQUEST")
}

func (h *LessonsCompletedHandler) handleValidationError(w http.ResponseWriter, err error) {
	errMsg := err.Error()
	if errMsg == "email é obrigatório" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "Parâmetro email é obrigatório", "MISSING_EMAIL")
		return
	}
	if errMsg == "page deve ser >= 1" || errMsg == "limit deve ser entre 1 e 100" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_PAGINATION")
		return
	}
	if errMsg == "data de início é obrigatória quando data final é fornecida" ||
		errMsg == "a data de início não pode ser maior que a data de fim" ||
		errMsg == "período máximo de 31 dias" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_DATE_RANGE")
		return
	}
	h.sendCustomErrorResponse(w, http.StatusBadRequest, errMsg, "INVALID_REQUEST")
}

func (h *LessonsCompletedHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	if errors.Is(err, lessons.ErrUserNotFoundOrNotInTenantForLessons) {
		h.sendCustomErrorResponse(w, http.StatusNotFound, "Usuário não encontrado ou não pertence a este tenant", "USER_NOT_IN_TENANT")
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

func (h *LessonsCompletedHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *LessonsCompletedHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := dto.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *LessonsCompletedHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
