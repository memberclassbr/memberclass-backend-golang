package ai

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/ai"
	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
)

type AILessonUseCase interface {
	GetLessons(ctx context.Context, req ai.GetAILessonsRequest) (*ai2.AILessonsResponse, error)
}
