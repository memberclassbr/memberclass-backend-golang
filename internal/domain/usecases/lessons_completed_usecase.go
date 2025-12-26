package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type LessonsCompletedUseCase struct {
	logger     ports.Logger
	userRepo   ports.UserRepository
	lessonRepo ports.LessonRepository
}

func NewLessonsCompletedUseCase(logger ports.Logger, userRepo ports.UserRepository, lessonRepo ports.LessonRepository) ports.LessonsCompletedUseCase {
	return &LessonsCompletedUseCase{
		logger:     logger,
		userRepo:   userRepo,
		lessonRepo: lessonRepo,
	}
}

var (
	ErrUserNotFoundOrNotInTenantForLessons = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
)

func (uc *LessonsCompletedUseCase) GetLessonsCompleted(ctx context.Context, req request.GetLessonsCompletedRequest, tenantID string) (*response.LessonsCompletedResponse, error) {
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

	pagination := response.Pagination{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  int(total),
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	return &response.LessonsCompletedResponse{
		OK: true,
		Data: response.LessonsCompletedData{
			CompletedLessons: lessons,
			Pagination:       pagination,
		},
	}, nil
}

