package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/ai"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAITenantUseCase_ProcessLessonsTenant(t *testing.T) {
	tests := []struct {
		name           string
		req            lesson.ProcessLessonsTenantRequest
		apiURL         string
		mockSetup      func(*mocks.MockTenantRepository, *mocks.MockAILessonUseCase, *mocks.MockLogger, *httptest.Server)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *lesson2.ProcessLessonsTenantResponse)
	}{
		{
			name: "should return error when tenantId is empty",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "",
			},
			mockSetup: func(*mocks.MockTenantRepository, *mocks.MockAILessonUseCase, *mocks.MockLogger, *httptest.Server) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "tenantId é obrigatório",
			},
		},
		{
			name: "should return error when TRANSCRIPTION_API_URL is not configured",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "tenant-123",
			},
			apiURL: "",
			mockSetup: func(*mocks.MockTenantRepository, *mocks.MockAILessonUseCase, *mocks.MockLogger, *httptest.Server) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "TRANSCRIPTION_API_URL não está configurada",
			},
		},
		{
			name: "should return error when tenant not found",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "non-existent",
			},
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockAILessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockTenantRepo.EXPECT().FindByID("non-existent").Return(nil, &memberclasserrors.MemberClassError{
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
			name: "should return error when AI is not enabled",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "tenant-123",
			},
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockAILessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&tenant.Tenant{
					ID:        "tenant-123",
					Name:      "Test Tenant",
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
			name: "should return false when no unprocessed lessons",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "tenant-123",
			},
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockAILessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&tenant.Tenant{
					ID:        "tenant-123",
					Name:      "Test Tenant",
					AIEnabled: true,
				}, nil)

				mockAILessonUseCase.EXPECT().GetLessons(mock.Anything, ai.GetAILessonsRequest{
					TenantID:        "tenant-123",
					OnlyUnprocessed: true,
				}).Return(&ai2.AILessonsResponse{
					Lessons:         []ai2.AILessonData{},
					Total:           0,
					TenantID:        "tenant-123",
					OnlyUnprocessed: true,
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *lesson2.ProcessLessonsTenantResponse) {
				assert.False(t, result.Success)
				assert.Equal(t, "Nenhuma lesson não processada encontrada para este tenant", result.Message)
				assert.Equal(t, 0, result.LessonsCount)
			},
		},
		{
			name: "should create job successfully",
			req: lesson.ProcessLessonsTenantRequest{
				TenantID: "tenant-123",
			},
			mockSetup: func(mockTenantRepo *mocks.MockTenantRepository, mockAILessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&tenant.Tenant{
					ID:        "tenant-123",
					Name:      "Test Tenant",
					AIEnabled: true,
				}, nil)

				mockAILessonUseCase.EXPECT().GetLessons(mock.Anything, ai.GetAILessonsRequest{
					TenantID:        "tenant-123",
					OnlyUnprocessed: true,
				}).Return(&ai2.AILessonsResponse{
					Lessons: []ai2.AILessonData{
						{
							ID:   "lesson-1",
							Name: "Lesson 1",
							Slug: "lesson-1",
						},
						{
							ID:   "lesson-2",
							Name: "Lesson 2",
							Slug: "lesson-2",
						},
					},
					Total:           2,
					TenantID:        "tenant-123",
					OnlyUnprocessed: true,
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *lesson2.ProcessLessonsTenantResponse) {
				assert.True(t, result.Success)
				assert.Equal(t, "Job de transcrição criado com sucesso", result.Message)
				assert.NotNil(t, result.JobID)
				assert.Equal(t, "job-123", *result.JobID)
				assert.Equal(t, 2, result.LessonsCount)
				assert.Equal(t, "tenant-123", result.TenantID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v2/extract-and-embed", r.URL.Path)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(dto.TranscriptionJobResponse{
					JobID:      "job-123",
					Status:     "accepted",
					VideoIDs:   []string{"lesson-1", "lesson-2"},
					QueuedJobs: 1,
					TraceID:    "trace-123",
				})
			}))
			defer server.Close()

			apiURL := server.URL
			if tt.apiURL == "" && tt.name == "should return error when TRANSCRIPTION_API_URL is not configured" {
				apiURL = ""
			}

			os.Setenv("TRANSCRIPTION_API_URL", apiURL)
			defer os.Unsetenv("TRANSCRIPTION_API_URL")

			mockTenantRepo := mocks.NewMockTenantRepository(t)
			mockAILessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			if apiURL == "" {
				mockLogger.EXPECT().Warn("TRANSCRIPTION_API_URL not configured").Maybe()
			}

			tt.mockSetup(mockTenantRepo, mockAILessonUseCase, mockLogger, server)

			useCase := NewAITenantUseCase(mockTenantRepo, mockAILessonUseCase, mockLogger)

			result, err := useCase.ProcessLessonsTenant(context.Background(), tt.req)

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

func TestAITenantUseCase_ProcessLessonsTenant_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "internal server error"}`)
	}))
	defer server.Close()

	os.Setenv("TRANSCRIPTION_API_URL", server.URL)
	defer os.Unsetenv("TRANSCRIPTION_API_URL")

	mockTenantRepo := mocks.NewMockTenantRepository(t)
	mockAILessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	mockTenantRepo.EXPECT().FindByID("tenant-123").Return(&tenant.Tenant{
		ID:        "tenant-123",
		Name:      "Test Tenant",
		AIEnabled: true,
	}, nil)

	mockAILessonUseCase.EXPECT().GetLessons(mock.Anything, ai.GetAILessonsRequest{
		TenantID:        "tenant-123",
		OnlyUnprocessed: true,
	}).Return(&ai2.AILessonsResponse{
		Lessons: []ai2.AILessonData{
			{
				ID:   "lesson-1",
				Name: "Lesson 1",
				Slug: "lesson-1",
			},
		},
		Total:           1,
		TenantID:        "tenant-123",
		OnlyUnprocessed: true,
	}, nil)

	useCase := NewAITenantUseCase(mockTenantRepo, mockAILessonUseCase, mockLogger)

	result, err := useCase.ProcessLessonsTenant(context.Background(), lesson.ProcessLessonsTenantRequest{
		TenantID: "tenant-123",
	})

	assert.Error(t, err)
	assert.Nil(t, result)

	var memberClassErr *memberclasserrors.MemberClassError
	assert.True(t, errors.As(err, &memberClassErr))
	assert.Equal(t, 500, memberClassErr.Code)
	assert.Contains(t, memberClassErr.Message, "Erro ao enviar para API de transcrição")
}
