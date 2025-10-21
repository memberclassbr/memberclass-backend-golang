package dto

type ProcessResult struct {
	Success        bool   `json:"success"`
	TotalPages     int    `json:"totalPages"`
	ProcessedPages int    `json:"processedPages"`
	Error          string `json:"error,omitempty"`
}

type BatchProcessResult struct {
	Processed int             `json:"processed"`
	Total     int             `json:"total"`
	Results   []ProcessResult `json:"results"`
}
