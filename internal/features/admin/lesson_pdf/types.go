package lesson_pdf

import "time"

// Lesson is the subset of the lesson row this slice reads. The PDF pipeline
// only cares about the media URL and the asset hanging off it; the rest is
// carried because the SELECT is shared with the detail read.
type Lesson struct {
	ID                     *string         `json:"id"`
	CreatedAt              time.Time       `json:"createdAt"`
	UpdatedAt              time.Time       `json:"updatedAt"`
	Access                 *int            `json:"access"`
	ReferenceAccess        *string         `json:"referenceAccess"`
	Type                   *string         `json:"type"`
	Slug                   *string         `json:"slug"`
	Name                   *string         `json:"name"`
	Published              bool            `json:"published"`
	Order                  *int            `json:"order"`
	MediaURL               *string         `json:"mediaUrl"`
	FullHDStatus           *string         `json:"fullHdStatus"`
	FullHDURL              *string         `json:"fullHdUrl"`
	FullHDRetries          *int            `json:"fullHdRetries"`
	Thumbnail              *string         `json:"thumbnail"`
	Content                *string         `json:"content"`
	ModuleID               *string         `json:"moduleId"`
	CreatedBy              *string         `json:"createdBy"`
	ShowDescriptionToggle  bool            `json:"showDescriptionToggle"`
	BannersTitle           *string         `json:"bannersTitle"`
	TranscriptionCompleted bool            `json:"transcriptionCompleted"`
	PDFAsset               *LessonPDFAsset `json:"pdfAsset,omitempty"`
}

// LessonPDFAsset tracks one lesson's rasterisation. Status moves
// pending → processing → done | partial | failed, and a failure that repeats
// is eventually marked permanently_failed by the cleanup action.
type LessonPDFAsset struct {
	ID           string          `json:"id"`
	LessonID     string          `json:"lessonId"`
	SourcePDFURL string          `json:"sourcePdfUrl"`
	TotalPages   *int            `json:"totalPages"`
	Status       string          `json:"status"`
	Error        *string         `json:"error"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	Pages        []LessonPDFPage `json:"pages,omitempty"`
}

// LessonPDFPage is one rendered page, stored in this deployment's bucket.
type LessonPDFPage struct {
	ID         string    `json:"id"`
	AssetID    string    `json:"assetId"`
	PageNumber int       `json:"pageNumber"`
	ImageURL   string    `json:"imageUrl"`
	Width      *int      `json:"width"`
	Height     *int      `json:"height"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ---------- request / response shapes ----------

type processLessonRequest struct {
	Action   string `json:"action" validate:"required"`
	LessonID string `json:"lessonId,omitempty"`
	Limit    *int   `json:"limit,omitempty"`
}

type processAllRequest struct {
	Limit *int `json:"limit,omitempty"`
}

type processResult struct {
	Success        bool   `json:"success"`
	TotalPages     int    `json:"totalPages"`
	ProcessedPages int    `json:"processedPages"`
	Error          string `json:"error,omitempty"`
}

type batchProcessResult struct {
	Processed int             `json:"processed"`
	Total     int             `json:"total"`
	Results   []processResult `json:"results"`
}

type processLessonResponse struct {
	Message string `json:"message"`
	Action  string `json:"action"`
	Result  any    `json:"result,omitempty"`
}

type processAllResponse struct {
	Message   string          `json:"message"`
	Processed int             `json:"processed"`
	Total     int             `json:"total"`
	Limit     *int            `json:"limit"`
	Success   bool            `json:"success"`
	Results   []processResult `json:"results"`
}

type regeneratePDFResponse struct {
	Message  string `json:"message"`
	LessonID string `json:"lessonId"`
	Status   string `json:"status"`
}

type cleanupFailedResult struct {
	AssetID  string `json:"assetId"`
	LessonID string `json:"lessonId"`
	Error    string `json:"error"`
}

type cleanupFailedResponse struct {
	Message string                `json:"message"`
	Removed int                   `json:"removed"`
	Total   int                   `json:"total"`
	Results []cleanupFailedResult `json:"results"`
}

type lessonPDFPageInfo struct {
	PageNumber int    `json:"pageNumber"`
	ImageURL   string `json:"imageUrl"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
}

type lessonPDFPagesResponse struct {
	LessonID   string              `json:"lessonId"`
	Status     string              `json:"status"`
	TotalPages int                 `json:"totalPages"`
	Pages      []lessonPDFPageInfo `json:"pages"`
}
