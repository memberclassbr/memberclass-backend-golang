package ai

import (
	"context"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/ai"
	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	ai3 "github.com/memberclass-backend-golang/internal/domain/ports/ai"
	lesson3 "github.com/memberclass-backend-golang/internal/domain/ports/lesson"
	"github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

// AILessonUseCaseImpl exposes the AI-dashboard lesson listing. The old
// UpdateTranscriptionStatus method moved into the transcription slice
// (it now writes the flag directly via raw SQL after the pgvector tx
// commits, and the manual PATCH endpoint lives on the slice for
// backwards compat).
type AILessonUseCaseImpl struct {
	lessonRepository lesson3.LessonRepository
	tenantRepository tenant.TenantRepository
	logger           ports.Logger
}

func NewAILessonUseCase(
	lessonRepository lesson3.LessonRepository,
	tenantRepository tenant.TenantRepository,
	logger ports.Logger,
) ai3.AILessonUseCase {
	return &AILessonUseCaseImpl{
		lessonRepository: lessonRepository,
		tenantRepository: tenantRepository,
		logger:           logger,
	}
}

func (uc *AILessonUseCaseImpl) GetLessons(ctx context.Context, req ai.GetAILessonsRequest) (*ai2.AILessonsResponse, error) {
	if req.TenantID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "tenantId é obrigatório",
		}
	}

	tenant, err := uc.tenantRepository.FindByID(req.TenantID)
	if err != nil {
		var memberClassErr *memberclasserrors.MemberClassError
		if errors.As(err, &memberClassErr) && memberClassErr.Code == 404 {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Tenant não encontrado",
			}
		}
		return nil, err
	}

	if !tenant.AIEnabled {
		return nil, &memberclasserrors.MemberClassError{
			Code:    403,
			Message: "IA não está habilitada para este tenant",
		}
	}

	lessons, err := uc.lessonRepository.GetLessonsWithHierarchyByTenant(ctx, req.TenantID, req.OnlyUnprocessed)
	if err != nil {
		return nil, err
	}

	lessonData := make([]ai2.AILessonData, len(lessons))
	for i, lesson := range lessons {
		lessonData[i] = ai2.AILessonData{
			ID:                     lesson.ID,
			Name:                   lesson.Name,
			Slug:                   lesson.Slug,
			Type:                   lesson.Type,
			MediaURL:               lesson.MediaURL,
			Thumbnail:              lesson.Thumbnail,
			Content:                lesson.Content,
			TranscriptionCompleted: lesson.TranscriptionCompleted,
			ModuleID:               lesson.ModuleID,
			ModuleName:             lesson.ModuleName,
			SectionID:              lesson.SectionID,
			SectionName:            lesson.SectionName,
			CourseID:               lesson.CourseID,
			CourseName:             lesson.CourseName,
			VitrineID:              lesson.VitrineID,
			VitrineName:            lesson.VitrineName,
		}
	}

	return &ai2.AILessonsResponse{
		Lessons:         lessonData,
		Total:           len(lessonData),
		TenantID:        req.TenantID,
		OnlyUnprocessed: req.OnlyUnprocessed,
	}, nil
}
