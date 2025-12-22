package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/usecases"
)

type UserPurchaseHandler struct {
	useCase ports.UserPurchaseUseCase
	logger  ports.Logger
}

func NewUserPurchaseHandler(useCase ports.UserPurchaseUseCase, logger ports.Logger) *UserPurchaseHandler {
	return &UserPurchaseHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *UserPurchaseHandler) GetUserPurchases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "email é obrigatório", "MISSING_EMAIL")
		return
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		var err error
		page, err = strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			h.sendErrorResponse(w, http.StatusBadRequest, "page must be a positive integer")
			return
		}
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 100 {
			h.sendErrorResponse(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
	}

	purchaseType := r.URL.Query().Get("type")

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendErrorResponse(w, http.StatusUnauthorized, "Tenant not found in context")
		return
	}

	req := request.GetUserPurchasesRequest{
		Email: email,
		Page:  page,
		Limit: limit,
		Type:  purchaseType,
	}

	response, err := h.useCase.GetUserPurchases(r.Context(), req, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *UserPurchaseHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	if errors.Is(err, usecases.ErrUserNotFoundOrNotInTenantForPurchases) {
		h.sendCustomErrorResponse(w, http.StatusNotFound, "Usuário não encontrado ou não pertence ao tenant autenticado", "USER_NOT_FOUND")
		return
	}

	errMsg := err.Error()
	if errMsg == "email é obrigatório" {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, "email é obrigatório", "MISSING_EMAIL")
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

func (h *UserPurchaseHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *UserPurchaseHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := dto.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *UserPurchaseHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
