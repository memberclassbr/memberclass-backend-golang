package ai

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
)

type AITenantUseCase interface {
	GetTenantsWithAIEnabled(ctx context.Context) (*ai.AITenantsResponse, error)
	ProcessLessonsTenant(ctx context.Context, req lesson.ProcessLessonsTenantRequest) (*lesson2.ProcessLessonsTenantResponse, error)
}
