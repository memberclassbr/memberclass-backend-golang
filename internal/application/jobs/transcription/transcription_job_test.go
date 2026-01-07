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
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

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
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.wantURL == "" {
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

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
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.mockSetup != nil {
				tt.mockSetup(mockTenantUseCase, mockLogger)
			}

			if tt.name == "should process tenants successfully" {
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", assert.AnError)
				mockLessonUseCase.EXPECT().GetLessons(mock.Anything, mock.Anything).Return(&response.AILessonsResponse{
					Total:   0,
					Lessons: []response.AILessonData{},
				}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)
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
		mockSetup func(*mocks.MockAILessonUseCase, *mocks.MockCache, *mocks.MockLogger, *httptest.Server)
		expectErr bool
	}{
		{
			name:   "should skip when tenant has pending job",
			tenant: response.AITenantData{ID: "tenant-1"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server *httptest.Server) {
				jobListData, _ := json.Marshal([]string{"job-1"})
				jobData := dto.TranscriptionJobData{
					JobID:    "job-1",
					TenantID: "tenant-1",
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
			},
			expectErr: false,
		},
		{
			name:   "should skip when no unprocessed lessons",
			tenant: response.AITenantData{ID: "tenant-1"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server *httptest.Server) {
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", nil)
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
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server *httptest.Server) {
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

				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", nil)
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

				jobListData, _ := json.Marshal([]string{})
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockCache.EXPECT().Set(mock.Anything, "transcription:job:job-123", mock.Anything, mock.Anything).Return(nil)
				mockCache.EXPECT().Set(mock.Anything, "transcription:jobs:list", mock.Anything, mock.Anything).Return(nil)

				os.Setenv("TRANSCRIPTION_API_URL", server.URL)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			var server *httptest.Server
			if tt.mockSetup != nil {
				tt.mockSetup(mockLessonUseCase, mockCache, mockLogger, server)
			}

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)
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

func TestTranscriptionJob_hasPendingJobForTenant(t *testing.T) {
	mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

	tests := []struct {
		name      string
		tenantID  string
		mockSetup func(*mocks.MockCache)
		expected  bool
	}{
		{
			name:     "should return false when job list is empty",
			tenantID: "tenant-1",
			mockSetup: func(mockCache *mocks.MockCache) {
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", nil)
			},
			expected: false,
		},
		{
			name:     "should return false when job list doesn't exist",
			tenantID: "tenant-1",
			mockSetup: func(mockCache *mocks.MockCache) {
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", assert.AnError)
			},
			expected: false,
		},
		{
			name:     "should return true when tenant has pending job",
			tenantID: "tenant-1",
			mockSetup: func(mockCache *mocks.MockCache) {
				jobListData, _ := json.Marshal([]string{"job-1"})
				jobData := dto.TranscriptionJobData{
					JobID:    "job-1",
					TenantID: "tenant-1",
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
			},
			expected: true,
		},
		{
			name:     "should return false when tenant doesn't have pending job",
			tenantID: "tenant-1",
			mockSetup: func(mockCache *mocks.MockCache) {
				jobListData, _ := json.Marshal([]string{"job-1"})
				jobData := dto.TranscriptionJobData{
					JobID:    "job-1",
					TenantID: "tenant-2",
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := mocks.NewMockCache(t)
			job.cache = mockCache

			if tt.mockSetup != nil {
				tt.mockSetup(mockCache)
			}

			result := job.hasPendingJobForTenant(context.Background(), tt.tenantID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTranscriptionJob_buildPayload(t *testing.T) {
	mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

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
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

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

func TestTranscriptionJob_saveJobToRedis(t *testing.T) {
	mockTenantUseCase := mocks.NewMockAITenantUseCase(t)
	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	os.Unsetenv("TRANSCRIPTION_API_URL")
	mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()

	job := NewTranscriptionJob(mockTenantUseCase, mockLessonUseCase, mockCache, mockLogger)

	lessons := []response.AILessonData{
		{ID: "lesson-1"},
		{ID: "lesson-2"},
	}

	jobListData, _ := json.Marshal([]string{})
	mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
	mockCache.EXPECT().Set(mock.Anything, "transcription:job:job-123", mock.Anything, mock.Anything).Return(nil)
	mockCache.EXPECT().Set(mock.Anything, "transcription:jobs:list", mock.Anything, mock.Anything).Return(nil)

	err := job.saveJobToRedis(context.Background(), "job-123", "tenant-1", lessons)
	assert.NoError(t, err)
}
