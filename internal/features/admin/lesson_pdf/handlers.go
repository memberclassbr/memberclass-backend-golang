package lesson_pdf

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ProcessLesson - POST /api/lessons/pdf-process
func (f *Feature) ProcessLesson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if !f.authorized(w, r) {
		return
	}

	var req processLessonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	switch req.Action {
	case "lesson":
		if req.LessonID == "" {
			writeError(w, http.StatusBadRequest, "Lesson ID is required for lesson action")
			return
		}
		result, err := f.processLesson(r.Context(), req.LessonID)
		if err != nil {
			f.writeUseCaseError(w, err)
			return
		}
		response := processLessonResponse{
			Message: "Lesson processing completed",
			Action:  "lesson",
			Result:  result,
		}
		writeJSON(w, http.StatusOK, response)

	case "retry":
		limit := 0
		if req.Limit != nil {
			limit = *req.Limit
		}
		result, err := f.retryFailedAssets(r.Context(), limit)
		if err != nil {
			f.writeUseCaseError(w, err)
			return
		}
		results := make([]processResult, len(result.Results))
		for i, res := range result.Results {
			results[i] = processResult{
				Success:        res.Success,
				TotalPages:     res.TotalPages,
				ProcessedPages: res.ProcessedPages,
				Error:          res.Error,
			}
		}
		response := processAllResponse{
			Message:   "Failed assets retry completed",
			Processed: result.Processed,
			Total:     result.Total,
			Limit:     req.Limit,
			Success:   result.Processed > 0,
			Results:   results,
		}
		writeJSON(w, http.StatusOK, response)

	case "cleanup":
		err := f.cleanupOrphanedPages(r.Context())
		if err != nil {
			f.writeUseCaseError(w, err)
			return
		}
		response := processLessonResponse{
			Message: "Cleanup completed",
			Action:  "cleanup",
		}
		writeJSON(w, http.StatusOK, response)

	case "cleanup-failed":
		result, err := f.cleanupPermanentlyFailedAssets(r.Context())
		if err != nil {
			f.writeUseCaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: retry, cleanup, cleanup-failed, or lesson")
	}
}

// ProcessAllPendingLessons - POST /api/lessons/process-all-pdfs
func (f *Feature) ProcessAllPendingLessons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if !f.authorized(w, r) {
		return
	}

	var req processAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body provided, use default limit
		req.Limit = nil
	}

	limit := 0
	if req.Limit != nil {
		limit = *req.Limit
	}

	result, err := f.processAllPending(r.Context(), limit)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	// Convert results to response format
	results := make([]processResult, len(result.Results))
	for i, res := range result.Results {
		results[i] = processResult{
			Success:        res.Success,
			TotalPages:     res.TotalPages,
			ProcessedPages: res.ProcessedPages,
			Error:          res.Error,
		}
	}

	response := processAllResponse{
		Message:   "PDF processing completed",
		Processed: result.Processed,
		Total:     result.Total,
		Limit:     req.Limit,
		Success:   result.Processed > 0,
		Results:   results,
	}

	writeJSON(w, http.StatusOK, response)
}

// RegeneratePDF - POST /api/lessons/{lessonId}/pdf-regenerate
func (f *Feature) RegeneratePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if !f.authorized(w, r) {
		return
	}

	// Extract lesson ID from Chi URL parameter
	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		writeError(w, http.StatusBadRequest, "Lesson ID is required")
		return
	}

	err := f.regeneratePDF(r.Context(), lessonID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	response := regeneratePDFResponse{
		Message:  "PDF regeneration queued successfully",
		LessonID: lessonID,
		Status:   "pending",
	}

	writeJSON(w, http.StatusOK, response)
}

// GetLessonsPage - GET /api/lessons/:lessonId/pdf-pages
func (f *Feature) GetLessonsPage(w http.ResponseWriter, r *http.Request) {

	if !f.authorized(w, r) {
		return
	}

	lessonID := chi.URLParam(r, "lessonId")
	if lessonID == "" {
		writeError(w, http.StatusBadRequest, "lessonId is required")
		return
	}

	lesson, err := f.getLessonWithPDFAsset(r.Context(), lessonID)
	if err != nil {
		f.log.Error("Error getting lesson with PDF asset: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Error fetching lesson")
		return
	}

	if lesson.PDFAsset == nil {

		response := lessonPDFPagesResponse{
			LessonID:   lessonID,
			Status:     "absent",
			TotalPages: 0,
			Pages:      []lessonPDFPageInfo{},
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	pages, err := f.GetPDFPagesByAssetID(r.Context(), lesson.PDFAsset.ID)
	if err != nil {
		f.log.Error("Error getting PDF pages: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Error fetching PDF pages")
		return
	}

	pageInfos := make([]lessonPDFPageInfo, len(pages))
	for i, page := range pages {
		pageInfos[i] = lessonPDFPageInfo{
			PageNumber: page.PageNumber,
			ImageURL:   page.ImageURL,
			Width:      page.Width,
			Height:     page.Height,
		}
	}

	totalPages := 0
	if lesson.PDFAsset.TotalPages != nil {
		totalPages = *lesson.PDFAsset.TotalPages
	}

	response := lessonPDFPagesResponse{
		LessonID:   lessonID,
		Status:     lesson.PDFAsset.Status,
		TotalPages: totalPages,
		Pages:      pageInfos,
	}

	writeJSON(w, http.StatusOK, response)
}
