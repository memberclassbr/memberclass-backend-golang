package lessons

import (
	"context"
	"errors"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	lesson3 "github.com/memberclass-backend-golang/internal/domain/ports/lesson"
	userports "github.com/memberclass-backend-golang/internal/domain/ports/user"
)

type LessonsCompletedUseCase struct {
	logger     ports.Logger
	userRepo   userports.UserRepository
	lessonRepo lesson3.LessonRepository
}

func NewLessonsCompletedUseCase(logger ports.Logger, userRepo userports.UserRepository, lessonRepo lesson3.LessonRepository) lesson3.LessonsCompletedUseCase {
	return &LessonsCompletedUseCase{
		logger:     logger,
		userRepo:   userRepo,
		lessonRepo: lessonRepo,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForLessons = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *LessonsCompletedUseCase) GetLessonsCompleted(ctx context.Context, req lesson.GetLessonsCompletedRequest, tenantID string) (*lesson2.LessonsCompletedResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := uc.userRepo.FindByEmail(req.Email)
	if err != nil {
		return nil, ErrUserNotFoundOrNotInTenantForLessons
	}

	belongs, err := uc.userRepo.BelongsToTenant(user.ID, tenantID)
	if err != nil {
		return nil, err
	}

	if !belongs {
		return nil, ErrUserNotFoundOrNotInTenantForLessons
	}

	now := time.Now()
	var startDate, endDate time.Time

	// Determine date range
	if req.StartDate == nil && req.EndDate == nil {
		// Default: last 31 days
		endDate = now
		startDate = now.AddDate(0, 0, -31)
	} else if req.StartDate != nil && req.EndDate == nil {
		// Only startDate: from start of day to end of day
		startDate = time.Date(req.StartDate.Year(), req.StartDate.Month(), req.StartDate.Day(), 0, 0, 0, 0, req.StartDate.Location())
		endDate = time.Date(req.StartDate.Year(), req.StartDate.Month(), req.StartDate.Day(), 23, 59, 59, 999999999, req.StartDate.Location())
	} else {
		// Both dates provided
		startDate = *req.StartDate
		endDate = *req.EndDate
	}

	lessons, total, err := uc.lessonRepo.FindCompletedLessonsByEmail(ctx, user.ID, tenantID, startDate, endDate, req.CourseID, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / req.Limit
	if int(total)%req.Limit > 0 {
		totalPages++
	}

	pagination := dto.PaginationMeta{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  total,
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	return &lesson2.LessonsCompletedResponse{
		OK: true,
		Data: lesson2.LessonsCompletedData{
			CompletedLessons: lessons,
			Pagination:       pagination,
		},
	}, nil
}
