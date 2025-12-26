package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type AILessonUseCase interface {
	UpdateTranscriptionStatus(ctx context.Context, lessonID string, req request.UpdateLessonTranscriptionRequest) (*response.LessonTranscriptionResponse, error)
}

