package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type LessonsCompletedUseCase interface {
	GetLessonsCompleted(ctx context.Context, req request.GetLessonsCompletedRequest, tenantID string) (*response.LessonsCompletedResponse, error)
}

