package student

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	student2 "github.com/memberclass-backend-golang/internal/domain/ports/student"
)

type StudentReportHandler struct {
	useCase student2.StudentReportUseCase
	logger  ports.Logger
}

func NewStudentReportHandler(useCase student2.StudentReportUseCase, logger ports.Logger) *StudentReportHandler {
	return &StudentReportHandler{
		useCase: useCase,
		logger:  logger,
	}
}

func (h *StudentReportHandler) GetStudentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	parsedReq, err := student.ParseStudentReportRequest(r.URL.Query())
	if err != nil {
		errorCode := "INVALID_PAGINATION"
		if err.Error() == "formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" {
			errorCode = "INVALID_DATE_FORMAT"
		} else if err.Error() == "formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" {
			errorCode = "INVALID_DATE_FORMAT"
		}
		h.sendCustomErrorResponse(w, http.StatusBadRequest, err.Error(), errorCode)
		return
	}

	if err := parsedReq.Validate(); err != nil {
		if err.Error() == "page deve ser >= 1" || err.Error() == "limit deve ser entre 1 e 100" {
			h.sendCustomErrorResponse(w, http.StatusBadRequest, "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100", "INVALID_PAGINATION")
			return
		}
		if err.Error() == "formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" {
			h.sendCustomErrorResponse(w, http.StatusBadRequest, "Formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)", "INVALID_DATE_FORMAT")
			return
		}
		if err.Error() == "formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" {
			h.sendCustomErrorResponse(w, http.StatusBadRequest, "Formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)", "INVALID_DATE_FORMAT")
			return
		}
		if err.Error() == "a data de início não pode ser maior que a data de fim" {
			h.sendCustomErrorResponse(w, http.StatusBadRequest, "startDate não pode ser maior que endDate", "INVALID_DATE_RANGE")
			return
		}
		h.sendCustomErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	response, err := h.useCase.GetStudentReport(r.Context(), *parsedReq, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *StudentReportHandler) GetStudentsRanking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	parsedReq, err := student.ParseStudentsRankingRequest(r.URL.Query())
	if err != nil {
		errorCode := "INVALID_REQUEST"
		if err.Error() == "limit deve ser um número" || err.Error() == "page deve ser um número" {
			errorCode = "INVALID_PAGINATION"
		}
		if err.Error() == "formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" || err.Error() == "formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)" {
			errorCode = "INVALID_DATE_FORMAT"
		}
		h.sendCustomErrorResponse(w, http.StatusBadRequest, err.Error(), errorCode)
		return
	}

	if err := parsedReq.Validate(); err != nil {
		h.sendCustomErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		h.sendCustomErrorResponse(w, http.StatusUnauthorized, "API key inválida", "INVALID_API_KEY")
		return
	}

	parsedReq.TenantID = tenant.ID
	response, fromCache, err := h.useCase.GetStudentsRanking(r.Context(), *parsedReq, tenant.ID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *StudentReportHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
		return
	}

	h.logger.Error("Unexpected error: " + err.Error())
	h.sendErrorResponse(w, http.StatusInternalServerError, "Erro interno do servidor")
}

func (h *StudentReportHandler) sendCustomErrorResponse(w http.ResponseWriter, statusCode int, errorMessage, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"ok":        false,
		"error":     errorMessage,
		"errorCode": errorCode,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *StudentReportHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := map[string]interface{}{
		"ok":        false,
		"error":     message,
		"errorCode": "INTERNAL_ERROR",
	}

	json.NewEncoder(w).Encode(response)
}

func (h *StudentReportHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
