package dto

type ProcessLessonRequest struct {
	Action   string `json:"action" validate:"required"`
	LessonID string `json:"lessonId,omitempty"`
}

type ProcessAllRequest struct {
	Limit *int `json:"limit,omitempty"`
}

type ProcessLessonResponse struct {
	Message string      `json:"message"`
	Action  string      `json:"action"`
	Result  interface{} `json:"result,omitempty"`
}

type ProcessAllResponse struct {
	Message   string                `json:"message"`
	Processed int                   `json:"processed"`
	Total     int                   `json:"total"`
	Limit     *int                  `json:"limit"`
	Success   bool                  `json:"success"`
	Results   []ProcessLessonResult `json:"results"`
}

type ProcessLessonResult struct {
	Success        bool   `json:"success"`
	TotalPages     int    `json:"totalPages"`
	ProcessedPages int    `json:"processedPages"`
	Error          string `json:"error,omitempty"`
}

type RegeneratePDFResponse struct {
	Message  string `json:"message"`
	LessonID string `json:"lessonId"`
	Status   string `json:"status"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
