package dto


type LessonPDFPagesResponse struct {
	LessonID   string                `json:"lessonId"`
	Status     string                `json:"status"`
	TotalPages int                   `json:"totalPages"`
	Pages      []LessonPDFPageInfo   `json:"pages"`
}


type LessonPDFPageInfo struct {
	PageNumber int    `json:"pageNumber"`
	ImageURL   string `json:"imageUrl"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
}
