package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type LessonHandler struct {
	useCase ports.PdfProcessorUseCase
	logger  ports.Logger
}

func NewLessonHandler(useCase ports.PdfProcessorUseCase, logger ports.Logger) *LessonHandler {
	return &LessonHandler{
		useCase: useCase,
		logger:  logger,
	}
}

// ProcessLesson - POST /api/lessons/pdf-process
func (h *LessonHandler) ProcessLesson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req dto.ProcessLessonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	switch req.Action {
	case "lesson":
		if req.LessonID == "" {
			h.sendErrorResponse(w, http.StatusBadRequest, "Lesson ID is required for lesson action")
			return
		}
		result, err := h.useCase.ProcessLesson(r.Context(), req.LessonID)
		if err != nil {
			h.handleUseCaseError(w, err)
			return
		}
		response := dto.ProcessLessonResponse{
			Message: "Lesson processing completed",
			Action:  "lesson",
			Result:  result,
		}
		h.sendJSONResponse(w, http.StatusOK, response)

	case "retry":
		err := h.useCase.RetryFailedAssets(r.Context())
		if err != nil {
			h.handleUseCaseError(w, err)
			return
		}
		response := dto.ProcessLessonResponse{
			Message: "Failed assets retry completed",
			Action:  "retry",
		}
		h.sendJSONResponse(w, http.StatusOK, response)

	case "cleanup":
		err := h.useCase.CleanupOrphanedPages(r.Context())
		if err != nil {
			h.handleUseCaseError(w, err)
			return
		}
		response := dto.ProcessLessonResponse{
			Message: "Cleanup completed",
			Action:  "cleanup",
		}
		h.sendJSONResponse(w, http.StatusOK, response)

	default:
		h.sendErrorResponse(w, http.StatusBadRequest, "Invalid action. Use: retry, cleanup, or lesson")
	}
}

// ProcessAllPendingLessons - POST /api/lessons/process-all-pdfs
func (h *LessonHandler) ProcessAllPendingLessons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req dto.ProcessAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body provided, use default limit
		req.Limit = nil
	}

	limit := 0
	if req.Limit != nil {
		limit = *req.Limit
	}

	result, err := h.useCase.ProcessAllPendingLessons(r.Context(), limit)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	// Convert results to response format
	results := make([]dto.ProcessLessonResult, len(result.Results))
	for i, res := range result.Results {
		results[i] = dto.ProcessLessonResult{
			Success:        res.Success,
			TotalPages:     res.TotalPages,
			ProcessedPages: res.ProcessedPages,
			Error:          res.Error,
		}
	}

	response := dto.ProcessAllResponse{
		Message:   "PDF processing completed",
		Processed: result.Processed,
		Total:     result.Total,
		Limit:     req.Limit,
		Success:   result.Processed > 0,
		Results:   results,
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

// RegeneratePDF - POST /api/lessons/{lessonId}/pdf-regenerate
func (h *LessonHandler) RegeneratePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract lesson ID from Chi URL parameter
	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "Lesson ID is required")
		return
	}

	err := h.useCase.RegeneratePDF(r.Context(), lessonID)
	if err != nil {
		h.handleUseCaseError(w, err)
		return
	}

	response := dto.RegeneratePDFResponse{
		Message:  "PDF regeneration queued successfully",
		LessonID: lessonID,
		Status:   "pending",
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (h *LessonHandler) handleUseCaseError(w http.ResponseWriter, err error) {
	var memberClassErr *memberclasserrors.MemberClassError
	if errors.As(err, &memberClassErr) {
		h.sendErrorResponse(w, memberClassErr.Code, memberClassErr.Message)
		return
	}

	h.logger.Error("Unexpected error: " + err.Error())
	h.sendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
}

func (h *LessonHandler) sendErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := dto.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

func (h *LessonHandler) sendJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
