package entities

import "time"

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

type LessonOnDelivery struct {
	DeliveryID string `json:"deliveryId"`
	LessonID   string `json:"lessonId"`
}
