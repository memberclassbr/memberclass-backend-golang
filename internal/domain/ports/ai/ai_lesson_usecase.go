package ai

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/ai"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
)

type AILessonUseCase interface {
	UpdateTranscriptionStatus(ctx context.Context, lessonID string, req lesson.UpdateLessonTranscriptionRequest) (*lesson2.LessonTranscriptionResponse, error)
	GetLessons(ctx context.Context, req ai.GetAILessonsRequest) (*ai2.AILessonsResponse, error)
}
