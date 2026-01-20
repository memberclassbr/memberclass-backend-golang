package lesson

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
)

type LessonsCompletedUseCase interface {
	GetLessonsCompleted(ctx context.Context, req lesson.GetLessonsCompletedRequest, tenantID string) (*lesson2.LessonsCompletedResponse, error)
}
