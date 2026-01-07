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
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAILessonUseCase(t *testing.T) {
	mockLessonRepo := mocks.NewMockLessonRepository(t)
	mockTenantRepo := mocks.NewMockTenantRepository(t)
	mockLogger := mocks.NewMockLogger(t)

	useCase := NewAILessonUseCase(mockLessonRepo, mockTenantRepo, mockLogger)

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

			mockTenantRepo := mocks.NewMockTenantRepository(t)
			useCase := NewAILessonUseCase(mockLessonRepo, mockTenantRepo, mockLogger)

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

func TestAILessonUseCase_GetLessons(t *testing.T) {
	tenantID := "tenant-123"
	lessonType := "video"
	mediaURL := "https://example.com/video.mp4"
	thumbnail := "https://example.com/thumb.jpg"
	content := "Test content"

	tests := []struct {
		name          string
		req           request.GetAILessonsRequest
		mockSetup     func(*mocks.MockLessonRepository, *mocks.MockTenantRepository, *mocks.MockLogger)
		expectError   bool
		expectedError *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *response.AILessonsResponse)
	}{
		{
			name: "should return error when tenantID is empty",
			req: request.GetAILessonsRequest{
				TenantID:        "",
				OnlyUnprocessed: false,
			},
			mockSetup: func(*mocks.MockLessonRepository, *mocks.MockTenantRepository, *mocks.MockLogger) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "tenantId é obrigatório",
			},
		},
		{
			name: "should return error when tenant not found",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Tenant não encontrado",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Tenant não encontrado",
			},
		},
		{
			name: "should return error when FindByID returns generic error",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name: "should return error when AI is not enabled",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(&entities.Tenant{
					ID:        tenantID,
					AIEnabled: false,
				}, nil)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    403,
				Message: "IA não está habilitada para este tenant",
			},
		},
		{
			name: "should return error when GetLessonsWithHierarchyByTenant fails",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(&entities.Tenant{
					ID:        tenantID,
					AIEnabled: true,
				}, nil)
				mockLessonRepo.EXPECT().GetLessonsWithHierarchyByTenant(mock.Anything, tenantID, false).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name: "should return success with all lessons",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(&entities.Tenant{
					ID:        tenantID,
					AIEnabled: true,
				}, nil)
				mockLessonRepo.EXPECT().GetLessonsWithHierarchyByTenant(mock.Anything, tenantID, false).Return([]ports.AILessonWithHierarchy{
					{
						ID:                     "lesson-1",
						Name:                   "Lesson 1",
						Slug:                   "lesson-1",
						Type:                   &lessonType,
						MediaURL:               &mediaURL,
						Thumbnail:              &thumbnail,
						Content:                &content,
						TranscriptionCompleted: true,
						ModuleID:               "module-1",
						ModuleName:              "Module 1",
						SectionID:              "section-1",
						SectionName:             "Section 1",
						CourseID:               "course-1",
						CourseName:              "Course 1",
						VitrineID:              "vitrine-1",
						VitrineName:             "Vitrine 1",
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AILessonsResponse) {
				assert.Equal(t, tenantID, result.TenantID)
				assert.False(t, result.OnlyUnprocessed)
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Lessons, 1)
				assert.Equal(t, "lesson-1", result.Lessons[0].ID)
				assert.Equal(t, "Lesson 1", result.Lessons[0].Name)
				assert.True(t, result.Lessons[0].TranscriptionCompleted)
			},
		},
		{
			name: "should return success with only unprocessed lessons",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: true,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(&entities.Tenant{
					ID:        tenantID,
					AIEnabled: true,
				}, nil)
				mockLessonRepo.EXPECT().GetLessonsWithHierarchyByTenant(mock.Anything, tenantID, true).Return([]ports.AILessonWithHierarchy{
					{
						ID:                     "lesson-2",
						Name:                   "Lesson 2",
						Slug:                   "lesson-2",
						TranscriptionCompleted: false,
						ModuleID:               "module-2",
						ModuleName:              "Module 2",
						SectionID:              "section-2",
						SectionName:             "Section 2",
						CourseID:               "course-2",
						CourseName:              "Course 2",
						VitrineID:              "vitrine-2",
						VitrineName:             "Vitrine 2",
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AILessonsResponse) {
				assert.Equal(t, tenantID, result.TenantID)
				assert.True(t, result.OnlyUnprocessed)
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Lessons, 1)
				assert.Equal(t, "lesson-2", result.Lessons[0].ID)
				assert.False(t, result.Lessons[0].TranscriptionCompleted)
			},
		},
		{
			name: "should return empty list when no lessons found",
			req: request.GetAILessonsRequest{
				TenantID:        tenantID,
				OnlyUnprocessed: false,
			},
			mockSetup: func(mockLessonRepo *mocks.MockLessonRepository, mockTenantRepo *mocks.MockTenantRepository, mockLogger *mocks.MockLogger) {
				mockTenantRepo.EXPECT().FindByID(tenantID).Return(&entities.Tenant{
					ID:        tenantID,
					AIEnabled: true,
				}, nil)
				mockLessonRepo.EXPECT().GetLessonsWithHierarchyByTenant(mock.Anything, tenantID, false).Return([]ports.AILessonWithHierarchy{}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *response.AILessonsResponse) {
				assert.Equal(t, tenantID, result.TenantID)
				assert.Equal(t, 0, result.Total)
				assert.Len(t, result.Lessons, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLessonRepo := mocks.NewMockLessonRepository(t)
			mockTenantRepo := mocks.NewMockTenantRepository(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockLessonRepo, mockTenantRepo, mockLogger)

			useCase := NewAILessonUseCase(mockLessonRepo, mockTenantRepo, mockLogger)

			result, err := useCase.GetLessons(context.Background(), tt.req)

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

