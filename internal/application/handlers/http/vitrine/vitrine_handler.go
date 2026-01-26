package vitrine

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	vitrineports "github.com/memberclass-backend-golang/internal/domain/ports/vitrine"
)

type VitrineHandler struct {
	useCase vitrineports.VitrineUseCase
	logger  ports.Logger
}

func NewVitrineHandler(useCase vitrineports.VitrineUseCase, logger ports.Logger) *VitrineHandler {
	return &VitrineHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *VitrineHandler) GetVitrines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetVitrines(r.Context(), tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *VitrineHandler) GetVitrine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	vitrineID := chi.URLParam(r, "vitrineId")
	if vitrineID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "vitrineId é obrigatório", "INVALID_REQUEST")
		return
	}

	includeChildren := h.parseIncludeChildren(r)

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetVitrine(r.Context(), vitrineID, tenant.ID, includeChildren)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *VitrineHandler) GetCourse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	courseID := chi.URLParam(r, "courseId")
	if courseID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "courseId é obrigatório", "INVALID_REQUEST")
		return
	}

	includeChildren := h.parseIncludeChildren(r)

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetCourse(r.Context(), courseID, tenant.ID, includeChildren)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *VitrineHandler) GetModule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	moduleID := chi.URLParam(r, "moduleId")
	if moduleID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "moduleId é obrigatório", "INVALID_REQUEST")
		return
	}

	includeChildren := h.parseIncludeChildren(r)

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetModule(r.Context(), moduleID, tenant.ID, includeChildren)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *VitrineHandler) GetLesson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "lessonId é obrigatório", "INVALID_REQUEST")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetLesson(r.Context(), lessonID, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *VitrineHandler) parseIncludeChildren(r *http.Request) bool {
	includeChildrenStr := r.URL.Query().Get("includeChildren")
	if includeChildrenStr == "" {
		return false
	}

	includeChildren, err := strconv.ParseBool(includeChildrenStr)
	if err != nil {
		return false
	}

	return includeChildren
}

func (h *VitrineHandler) handleUseCaseError(w http.ResponseWriter, err error) {
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
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, memberClassErr.Message, "INVALID_API_KEY")
	case 404:
		h.sendCustomErrorResponse(w, http.StatusNotFound, memberClassErr.Message, "NOT_FOUND")
	default:
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
	}
}

func (h *VitrineHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *VitrineHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *VitrineHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
