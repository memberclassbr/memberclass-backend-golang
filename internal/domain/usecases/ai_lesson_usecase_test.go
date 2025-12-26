package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAILessonUseCase(t *testing.T) {
	mockLessonRepo := mocks.NewMockLessonRepository(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewAILessonUseCase(mockLessonRepo, mockLogger)

	assert.NotNil(t, useCase)
}

func TestAILessonUseCase_UpdateTranscriptionStatus(t *testing.T) {
	lessonID := "lesson-123"
	lessonName := "Test Lesson"
	lessonSlug := "test-lesson"

	tests := []struct {
		name          string
		lessonID      string
		req           request.UpdateLessonTranscriptionRequest
		mockSetup     func(*mocks.MockLessonRepository, *mocks.MockLogger)
		expectError   bool
		expectedError *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *response.LessonTranscriptionResponse)
	}{
		{
			name:     "should return error when lessonID is empty",
			lessonID: "",
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(*mocks.MockLessonRepository, *mocks.MockLogger) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "lessonId é obrigatório",
			},
		},
		{
			name:     "should return error when lesson not found",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(nil, nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Aula não encontrada",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Aula não encontrada",
			},
		},
		{
			name:     "should return error when GetByIDWithTenant returns generic error",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(nil, nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "should return error when AI is not enabled",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(
					&entities.Lesson{
						ID:                     &lessonID,
						Name:                   &lessonName,
						Slug:                   &lessonSlug,
						TranscriptionCompleted: false,
						UpdatedAt:              time.Now(),
					},
					&entities.Tenant{
						ID:        "tenant-123",
						AIEnabled: false,
					},
					nil,
				)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    403,
				Message: "IA não está habilitada para este tenant",
			},
		},
		{
			name:     "should return error when UpdateTranscriptionStatus fails",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(
					&entities.Lesson{
						ID:                     &lessonID,
						Name:                   &lessonName,
						Slug:                   &lessonSlug,
						TranscriptionCompleted: false,
						UpdatedAt:              time.Now(),
					},
					&entities.Tenant{
						ID:        "tenant-123",
						AIEnabled: true,
					},
					nil,
				)
				mockLessonRepo.EXPECT().UpdateTranscriptionStatus(mock.Anything, lessonID, true).Return(errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "should return success when updating transcription status",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(
					&entities.Lesson{
						ID:                     &lessonID,
						Name:                   &lessonName,
						Slug:                   &lessonSlug,
						TranscriptionCompleted: false,
						UpdatedAt:              time.Now(),
					},
					&entities.Tenant{
						ID:        "tenant-123",
						AIEnabled: true,
					},
					nil,
				)
				mockLessonRepo.EXPECT().UpdateTranscriptionStatus(mock.Anything, lessonID, true).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.LessonTranscriptionResponse) {
				assert.Equal(t, lessonID, result.Lesson.ID)
				assert.Equal(t, lessonName, result.Lesson.Name)
				assert.Equal(t, lessonSlug, result.Lesson.Slug)
				assert.True(t, result.Lesson.TranscriptionCompleted)
				assert.Equal(t, "Status de transcrição atualizado com sucesso", result.Message)
			},
		},
		{
			name:     "should return success when setting transcription to false",
			lessonID: lessonID,
			req:      request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: false},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockLogger *mocks.MockLogger) {
				mockLessonRepo.EXPECT().GetByIDWithTenant(mock.Anything, lessonID).Return(
					&entities.Lesson{
						ID:                     &lessonID,
						Name:                   &lessonName,
						Slug:                   &lessonSlug,
						TranscriptionCompleted: true,
						UpdatedAt:              time.Now(),
					},
					&entities.Tenant{
						ID:        "tenant-123",
						AIEnabled: true,
					},
					nil,
				)
				mockLessonRepo.EXPECT().UpdateTranscriptionStatus(mock.Anything, lessonID, false).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.LessonTranscriptionResponse) {
				assert.Equal(t, lessonID, result.Lesson.ID)
				assert.False(t, result.Lesson.TranscriptionCompleted)
				assert.Equal(t, "Status de transcrição atualizado com sucesso", result.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLessonRepo := mocks.NewMockLessonRepository(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockLessonRepo, mockLogger)

			useCase := NewAILessonUseCase(mockLessonRepo, mockLogger)

			result, err := useCase.UpdateTranscriptionStatus(context.Background(), tt.lessonID, tt.req)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						if tt.expectedError.Message != "" {
							assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
						}
					}
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

