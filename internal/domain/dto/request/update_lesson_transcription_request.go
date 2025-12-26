package request

type UpdateLessonTranscriptionRequest struct {
	TranscriptionCompleted bool `json:"transcriptionCompleted"`
}

func (r *UpdateLessonTranscriptionRequest) Validate() error {
	return nil
}

