package transcription

import (
	"net/http"
)

type transcriptionStatsResponse struct {
	TenantID    string `json:"tenantId"`
	CourseID    string `json:"courseId,omitempty"`
	ModuleID    string `json:"moduleId,omitempty"`
	Total       int    `json:"total"`
	Transcribed int    `json:"transcribed"`
	Pending     int    `json:"pending"`
}

// GetTranscriptionStats handles `GET /api/v1/ai/transcription-stats`.
//
// Query params:
//   - tenantId  (required)
//   - courseId  (optional)
//   - moduleId  (optional, takes precedence over courseId — scope follows
//     the hierarchy Lesson → Module → Section → Course → Vitrine → Tenant
//     so a moduleId already pins a course)
//
// Only counts Bunny-backed, published lessons (the population the
// pipeline can actually transcribe). transcriptionCompleted=NULL is
// treated as false.
func (f *Feature) GetTranscriptionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), "")
		return
	}
	if !f.requireInternalAPIKey(w, r) {
		return
	}

	tenantID := r.URL.Query().Get("tenantId")
	if tenantID == "" {
		writeCustomError(w, http.StatusBadRequest, "tenantId é obrigatório", "MISSING_TENANT_ID")
		return
	}
	courseID := r.URL.Query().Get("courseId")
	moduleID := r.URL.Query().Get("moduleId")

	var stats transcriptionStatsResponse
	stats.TenantID = tenantID
	stats.CourseID = courseID
	stats.ModuleID = moduleID

	row := f.memberclassDB.QueryRowContext(r.Context(),
		sqlTranscriptionStats, tenantID, courseID, moduleID)
	if err := row.Scan(&stats.Total, &stats.Transcribed, &stats.Pending); err != nil {
		f.log.Error("transcription.stats.scan_failed",
			"tenant", tenantID, "course", courseID, "module", moduleID,
			"error", err.Error())
		writeError(w, http.StatusInternalServerError, "Internal Server Error", "")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
