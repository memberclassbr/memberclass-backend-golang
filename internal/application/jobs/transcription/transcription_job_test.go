package transcription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTranscriptionJob_Name(t *testing.T) {
	mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)

	assert.Equal(t, "transcription-job", job.Name())
}

func TestNewTranscriptionJob(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	tests := []struct {
		name     string
		apiURL   string
		wantURL  string
		setupEnv func()
	}{
		{
			name:    "should create job with API URL from env",
			apiURL:  "https://api.example.com",
			wantURL: "https://api.example.com",
			setupEnv: func() {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			},
		},
		{
			name:    "should create job with empty API URL when env not set",
			apiURL:  "",
			wantURL: "",
			setupEnv: func() {
				os.Unsetenv("TRANSCRIPTION_API_URL")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.wantURL == "" {
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)

			assert.NotNil(t, job)
			assert.Equal(t, tt.wantURL, job.apiURL)
		})
	}
}

func TestTranscriptionJob_Execute(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	tests := []struct {
		name        string
		apiURL      string
		mockSetup   func(*mocks.MockAITenantUseCase, *mocks.MockLogger)
		expectError bool
		setupEnv    func()
	}{
		{
			name:   "should skip execution when API URL is not configured",
			apiURL: "",
			setupEnv: func() {
				os.Unsetenv("TRANSCRIPTION_API_URL")
			},
			mockSetup: func(mockTenantUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured, skipping job execution").Return()
			},
			expectError: false,
		},
		{
			name:   "should return error when fetching tenants fails",
			apiURL: "https://api.example.com",
			setupEnv: func() {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			},
			mockSetup: func(mockTenantUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				mockTenantUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(nil, assert.AnError)
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
			},
			expectError: true,
		},
		{
			name:   "should process tenants successfully",
			apiURL: "https://api.example.com",
			setupEnv: func() {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			},
			mockSetup: func(mockTenantUseCase *mocks.MockAITenantUseCase, mockLogger *mocks.MockLogger) {
				tenants := &response.AITenantsResponse{
					Total:   1,
					Tenants: []response.AITenantData{{ID: "tenant-1"}},
				}
				mockTenantUseCase.EXPECT().GetTenantsWithAIEnabled(mock.Anything).Return(tenants, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.mockSetup != nil {
				tt.mockSetup(mockTenantUseCase, mockLogger)
			}

			if tt.name == "should process tenants successfully" {
				mockLessonUseCase.EXPECT().GetLessons(mock.Anything, mock.Anything).Return(&response.AILessonsResponse{
					Total:   0,
					Lessons: []response.AILessonData{},
				}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)
			err := job.Execute(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTranscriptionJob_processTenant(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")

	tests := []struct {
		name      string
		tenant    response.AITenantData
		mockSetup func(*mocks.MockAILessonUseCase, *mocks.MockLogger, *httptest.Server)
		expectErr bool
	}{
		{
			name:   "should skip when no unprocessed lessons",
			tenant: response.AITenantData{ID: "tenant-1"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockLessonUseCase.EXPECT().GetLessons(mock.Anything, request.GetAILessonsRequest{
					TenantID:        "tenant-1",
					OnlyUnprocessed: true,
				}).Return(&response.AILessonsResponse{Total: 0}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
			},
			expectErr: false,
		},
		{
			name:   "should process tenant successfully",
			tenant: response.AITenantData{ID: "tenant-1"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger, server *httptest.Server) {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/extract-and-embed", r.URL.Path)
					assert.Equal(t, http.MethodPost, r.Method)

					var payload dto.TranscriptionJobRequest
					json.NewDecoder(r.Body).Decode(&payload)
					assert.Equal(t, "tenant-1", payload.TenantID)
					assert.Len(t, payload.Lessons, 1)

					response := dto.TranscriptionJobResponse{
						JobID:  "job-123",
						Status: "pending",
					}
					w.WriteHeader(http.StatusAccepted)
					json.NewEncoder(w).Encode(response)
				}))

				mockLessonUseCase.EXPECT().GetLessons(mock.Anything, request.GetAILessonsRequest{
					TenantID:        "tenant-1",
					OnlyUnprocessed: true,
				}).Return(&response.AILessonsResponse{
					Total: 1,
					Lessons: []response.AILessonData{
						{
							ID:                     "lesson-1",
							Name:                   "Test Lesson",
							Slug:                   "test-lesson",
							TranscriptionCompleted: false,
						},
					},
				}, nil)

				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return().Times(2)

				os.Setenv("TRANSCRIPTION_API_URL", server.URL)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			var server *httptest.Server
			if tt.mockSetup != nil {
				tt.mockSetup(mockLessonUseCase, mockLogger, server)
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)
			err := job.processTenant(context.Background(), tt.tenant)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if server != nil {
				server.Close()
			}
		})
	}
}

func TestTranscriptionJob_buildPayload(t *testing.T) {
	mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)

	lessonType := "video"
	mediaURL := "https://example.com/video.mp4"
	lessons := []response.AILessonData{
		{
			ID:                     "lesson-1",
			Name:                   "Test Lesson",
			Slug:                   "test-lesson",
			Type:                   &lessonType,
			MediaURL:               &mediaURL,
			TranscriptionCompleted: false,
			ModuleID:               "module-1",
			ModuleName:             "Module 1",
			SectionID:              "section-1",
			SectionName:            "Section 1",
			CourseID:               "course-1",
			CourseName:             "Course 1",
			VitrineID:              "vitrine-1",
			VitrineName:            "Vitrine 1",
		},
	}

	payload := job.buildPayload(lessons, "tenant-1")

	assert.Equal(t, "tenant-1", payload.TenantID)
	assert.Len(t, payload.Lessons, 1)
	assert.Equal(t, "lesson-1", payload.Lessons[0].ID)
	assert.Equal(t, "Test Lesson", payload.Lessons[0].Name)
	assert.Equal(t, "test-lesson", payload.Lessons[0].Slug)
	assert.Equal(t, &lessonType, payload.Lessons[0].Type)
	assert.Equal(t, &mediaURL, payload.Lessons[0].MediaURL)
	assert.False(t, payload.Lessons[0].TranscriptionCompleted)
}

func TestTranscriptionJob_sendToAPI(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	tests := []struct {
		name           string
		responseStatus int
		responseBody   interface{}
		expectError    bool
	}{
		{
			name:           "should send payload successfully",
			responseStatus: http.StatusAccepted,
			responseBody: dto.TranscriptionJobResponse{
				JobID:  "job-123",
				Status: "pending",
			},
			expectError: false,
		},
		{
			name:           "should return error on API error",
			responseStatus: http.StatusBadRequest,
			responseBody:   map[string]string{"error": "invalid payload"},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			os.Setenv("TRANSCRIPTION_API_URL", server.URL)

			mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockLogger)

			payload := dto.TranscriptionJobRequest{
				TenantID: "tenant-1",
				Lessons:  []dto.TranscriptionLessonData{},
			}

			result, err := job.sendToAPI(context.Background(), payload)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "job-123", result.JobID)
			}
		})
	}
}
