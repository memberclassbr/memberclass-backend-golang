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

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func TestTranscriptionStatusCheckerJob_Name(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")

	mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
	mockCache := mocks.NewMockCache(t)
	mockLogger := mocks.NewMockLogger(t)

	job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)

	assert.Equal(t, "transcription-status-checker-job", job.Name())
}

func TestNewTranscriptionStatusCheckerJob(t *testing.T) {
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

			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.wantURL == "" {
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()
			}

			job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)

			assert.NotNil(t, job)
			assert.Equal(t, tt.wantURL, job.apiURL)
		})
	}
}

func TestTranscriptionStatusCheckerJob_Execute(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	tests := []struct {
		name      string
		setupEnv  func()
		mockSetup func(*mocks.MockCache, *mocks.MockLogger)
		expectErr bool
	}{
		{
			name: "should skip execution when API URL is not configured",
			setupEnv: func() {
				os.Unsetenv("TRANSCRIPTION_API_URL")
			},
			mockSetup: func(mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured").Return()
				mockLogger.EXPECT().Error("TRANSCRIPTION_API_URL not configured, skipping job execution").Return()
			},
			expectErr: false,
		},
		{
			name: "should return early when no pending jobs",
			setupEnv: func() {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			},
			mockSetup: func(mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return("", &mockError{msg: "redis: nil"})
				mockLogger.EXPECT().Info("No pending jobs to check").Return()
			},
			expectErr: false,
		},
		{
			name: "should process jobs successfully",
			setupEnv: func() {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			},
			mockSetup: func(mockCache *mocks.MockCache, mockLogger *mocks.MockLogger) {
				jobListData, _ := json.Marshal([]string{"job-1"})
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return("", &mockError{msg: "redis: nil"})
				mockCache.EXPECT().Exists(mock.Anything, "transcription:job:job-1").Return(false, nil)
				mockCache.EXPECT().Delete(mock.Anything, "transcription:jobs:list").Return(nil)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			if tt.mockSetup != nil {
				tt.mockSetup(mockCache, mockLogger)
			}

			job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)
			err := job.Execute(context.Background())

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTranscriptionStatusCheckerJob_checkJobStatus(t *testing.T) {
	originalURL := os.Getenv("TRANSCRIPTION_API_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("TRANSCRIPTION_API_URL", originalURL)
		} else {
			os.Unsetenv("TRANSCRIPTION_API_URL")
		}
	}()

	tests := []struct {
		name      string
		jobID     string
		mockSetup func(*mocks.MockAILessonUseCase, *mocks.MockCache, *mocks.MockLogger, **httptest.Server)
		expectErr bool
	}{
		{
			name:  "should return nil when job data doesn't exist",
			jobID: "job-1",
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return("", &mockError{msg: "redis: nil"})
			},
			expectErr: false,
		},
		{
			name:  "should update lesson when status is COMPLETED",
			jobID: "job-1",
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/jobs/job-1/status", r.URL.Path)
					statusResponse := dto.TranscriptionJobStatusResponse{
						JobID:    "job-1",
						Status:   "processing",
						Lessons: []dto.TranscriptionLessonStatus{
							{
								LessonID: "lesson-1",
								Status:   "COMPLETED",
							},
						},
					}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(statusResponse)
				}))
				*server = s

				jobData := dto.TranscriptionJobData{
					JobID:     "job-1",
					TenantID:  "tenant-1",
					LessonIDs: []string{"lesson-1"},
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLessonUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-1",
					request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(&response.LessonTranscriptionResponse{}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return().Times(2)
				mockCache.EXPECT().Delete(mock.Anything, "transcription:job:job-1").Return(nil)

				os.Setenv("TRANSCRIPTION_API_URL", s.URL)
			},
			expectErr: false,
		},
		{
			name:  "should log error when lesson status is FAILED",
			jobID: "job-1",
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					errorMsg := "transcription failed"
					statusResponse := dto.TranscriptionJobStatusResponse{
						JobID:    "job-1",
						Status:   "processing",
						Lessons: []dto.TranscriptionLessonStatus{
							{
								LessonID:     "lesson-1",
								Status:       "FAILED",
								ErrorMessage: &errorMsg,
							},
						},
					}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(statusResponse)
				}))
				*server = s

				jobData := dto.TranscriptionJobData{
					JobID:     "job-1",
					TenantID:  "tenant-1",
					LessonIDs: []string{"lesson-1"},
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLogger.EXPECT().Error(mock.AnythingOfType("string")).Return()
				mockCache.EXPECT().Delete(mock.Anything, "transcription:job:job-1").Return(nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()

				os.Setenv("TRANSCRIPTION_API_URL", s.URL)
			},
			expectErr: false,
		},
		{
			name:  "should delete job from Redis when all lessons completed",
			jobID: "job-1",
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					statusResponse := dto.TranscriptionJobStatusResponse{
						JobID:    "job-1",
						Status:   "completed",
						Lessons: []dto.TranscriptionLessonStatus{
							{
								LessonID: "lesson-1",
								Status:   "COMPLETED",
							},
						},
					}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(statusResponse)
				}))
				*server = s

				jobData := dto.TranscriptionJobData{
					JobID:     "job-1",
					TenantID:  "tenant-1",
					LessonIDs: []string{"lesson-1"},
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLessonUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-1",
					request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(&response.LessonTranscriptionResponse{}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return().Times(2)
				mockCache.EXPECT().Delete(mock.Anything, "transcription:job:job-1").Return(nil)

				os.Setenv("TRANSCRIPTION_API_URL", s.URL)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalURL := os.Getenv("TRANSCRIPTION_API_URL")
			defer func() {
				if originalURL != "" {
					os.Setenv("TRANSCRIPTION_API_URL", originalURL)
				} else {
					os.Unsetenv("TRANSCRIPTION_API_URL")
				}
			}()

			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			var server *httptest.Server
			if tt.mockSetup != nil {
				tt.mockSetup(mockLessonUseCase, mockCache, mockLogger, &server)
			}

			if server == nil {
				os.Setenv("TRANSCRIPTION_API_URL", "https://api.example.com")
			}

			job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)
			err := job.checkJobStatus(context.Background(), tt.jobID)

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

func TestTranscriptionStatusCheckerJob_getJobStatusFromAPI(t *testing.T) {
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
		jobID          string
		responseStatus int
		responseBody   interface{}
		expectError    bool
	}{
		{
			name:  "should get job status successfully",
			jobID: "job-123",
			responseStatus: http.StatusOK,
			responseBody: dto.TranscriptionJobStatusResponse{
				JobID:    "job-123",
				Status:   "processing",
				Progress: 50,
				Total:    100,
				Lessons: []dto.TranscriptionLessonStatus{
					{
						LessonID: "lesson-1",
						Status:   "PROCESSING",
					},
				},
			},
			expectError: false,
		},
		{
			name:  "should return error on API error",
			jobID: "job-123",
			responseStatus: http.StatusNotFound,
			responseBody:   map[string]string{"error": "job not found"},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/jobs/"+tt.jobID+"/status", r.URL.Path)
				w.WriteHeader(tt.responseStatus)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			os.Setenv("TRANSCRIPTION_API_URL", server.URL)

			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)

			result, err := job.getJobStatusFromAPI(context.Background(), tt.jobID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.jobID, result.JobID)
			}
		})
	}
}

func TestTranscriptionStatusCheckerJob_Execute_WithJobList(t *testing.T) {
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
		jobList   []string
		mockSetup func(*mocks.MockAILessonUseCase, *mocks.MockCache, *mocks.MockLogger, **httptest.Server)
		expectErr bool
	}{
		{
			name:    "should update job list when jobs are completed",
			jobList: []string{"job-1", "job-2"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/jobs/job-1/status" {
						statusResponse := dto.TranscriptionJobStatusResponse{
							JobID:    "job-1",
							Status:   "completed",
							Lessons: []dto.TranscriptionLessonStatus{
								{
									LessonID: "lesson-1",
									Status:   "COMPLETED",
								},
							},
						}
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(statusResponse)
					} else if r.URL.Path == "/api/jobs/job-2/status" {
						statusResponse := dto.TranscriptionJobStatusResponse{
							JobID:    "job-2",
							Status:   "processing",
							Lessons: []dto.TranscriptionLessonStatus{
								{
									LessonID: "lesson-2",
									Status:   "PROCESSING",
								},
							},
						}
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(statusResponse)
					}
				}))
				*server = s

				jobListData, _ := json.Marshal([]string{"job-1", "job-2"})
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()

				jobData := dto.TranscriptionJobData{
					JobID:     "job-1",
					TenantID:  "tenant-1",
					LessonIDs: []string{"lesson-1"},
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLessonUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-1",
					request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(&response.LessonTranscriptionResponse{}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockCache.EXPECT().Delete(mock.Anything, "transcription:job:job-1").Return(nil)
				mockCache.EXPECT().Exists(mock.Anything, "transcription:job:job-1").Return(false, nil)

				jobData2 := dto.TranscriptionJobData{
					JobID:     "job-2",
					TenantID:  "tenant-2",
					LessonIDs: []string{"lesson-2"},
				}
				jobData2JSON, _ := json.Marshal(jobData2)
				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-2").Return(string(jobData2JSON), nil)
				mockCache.EXPECT().Exists(mock.Anything, "transcription:job:job-2").Return(true, nil)

				mockCache.EXPECT().Set(mock.Anything, "transcription:jobs:list", mock.Anything, mock.Anything).Return(nil)

				os.Setenv("TRANSCRIPTION_API_URL", s.URL)
			},
			expectErr: false,
		},
		{
			name:    "should delete job list when all jobs are completed",
			jobList: []string{"job-1"},
			mockSetup: func(mockLessonUseCase *mocks.MockAILessonUseCase, mockCache *mocks.MockCache, mockLogger *mocks.MockLogger, server **httptest.Server) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					statusResponse := dto.TranscriptionJobStatusResponse{
						JobID:    "job-1",
						Status:   "completed",
						Lessons: []dto.TranscriptionLessonStatus{
							{
								LessonID: "lesson-1",
								Status:   "COMPLETED",
							},
						},
					}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(statusResponse)
				}))
				*server = s

				jobListData, _ := json.Marshal([]string{"job-1"})
				mockCache.EXPECT().Get(mock.Anything, "transcription:jobs:list").Return(string(jobListData), nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()

				jobData := dto.TranscriptionJobData{
					JobID:     "job-1",
					TenantID:  "tenant-1",
					LessonIDs: []string{"lesson-1"},
				}
				jobDataJSON, _ := json.Marshal(jobData)

				mockCache.EXPECT().Get(mock.Anything, "transcription:job:job-1").Return(string(jobDataJSON), nil)
				mockLessonUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-1",
					request.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(&response.LessonTranscriptionResponse{}, nil)
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockLogger.EXPECT().Info(mock.AnythingOfType("string")).Return()
				mockCache.EXPECT().Delete(mock.Anything, "transcription:job:job-1").Return(nil)
				mockCache.EXPECT().Exists(mock.Anything, "transcription:job:job-1").Return(false, nil)
				mockCache.EXPECT().Delete(mock.Anything, "transcription:jobs:list").Return(nil)

				os.Setenv("TRANSCRIPTION_API_URL", s.URL)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLessonUseCase := mocks.NewMockAILessonUseCase(t)
			mockCache := mocks.NewMockCache(t)
			mockLogger := mocks.NewMockLogger(t)

			var server *httptest.Server
			if tt.mockSetup != nil {
				tt.mockSetup(mockLessonUseCase, mockCache, mockLogger, &server)
			}

			job := NewTranscriptionStatusCheckerJob(mockLessonUseCase, mockCache, mockLogger)
			err := job.Execute(context.Background())

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

