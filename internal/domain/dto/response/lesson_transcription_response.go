package response

import "time"

type LessonTranscriptionResponse struct {
	Lesson  LessonTranscriptionData `json:"lesson"`
	Message string                  `json:"message"`
}

type LessonTranscriptionData struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Slug                   string    `json:"slug"`
	TranscriptionCompleted bool      `json:"transcriptionCompleted"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

