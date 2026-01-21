package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/ai"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/lesson"
	ai2 "github.com/memberclass-backend-golang/internal/domain/dto/response/ai"
	lesson2 "github.com/memberclass-backend-golang/internal/domain/dto/response/lesson"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewAILessonHandler(t *testing.T) {
	mockUseCase := mocks.NewMockAILessonUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewAILessonHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestAILessonHandler_UpdateTranscriptionStatus(t *testing.T) {
	os.Setenv("INTERNAL_AI_API_KEY", "test-api-key")
	defer os.Unsetenv("INTERNAL_AI_API_KEY")

	tests := []struct {
		name             string
		method           string
		lessonID         string
		apiKey           string
		requestBody      interface{}
		mockSetup        func(*mocks.MockAILessonUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:     "should return method not allowed for GET",
			method:   http.MethodGet,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusMethodNotAllowed,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Method Not Allowed", response["error"])
				assert.Equal(t, "Method not allowed", response["message"])
			},
		},
		{
			name:     "should return unauthorized when api key is missing",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:     "should return unauthorized when api key is invalid",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "wrong-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:     "should return bad request when lessonId is empty",
			method:   http.MethodPatch,
			lessonID: "",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "lessonId é obrigatório", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:        "should return bad request when body is invalid JSON",
			method:      http.MethodPatch,
			lessonID:    "lesson-123",
			apiKey:      "test-api-key",
			requestBody: "invalid json",
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "transcriptionCompleted deve ser um booleano", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:     "should return success when transcription status is updated",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(&lesson2.LessonTranscriptionResponse{
					Lesson: lesson2.LessonTranscriptionData{
						ID:                     "lesson-123",
						Name:                   "Test Lesson",
						Slug:                   "test-lesson",
						TranscriptionCompleted: true,
						UpdatedAt:              time.Now(),
					},
					Message: "Status de transcrição atualizado com sucesso",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response lesson2.LessonTranscriptionResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "lesson-123", response.Lesson.ID)
				assert.True(t, response.Lesson.TranscriptionCompleted)
				assert.Equal(t, "Status de transcrição atualizado com sucesso", response.Message)
			},
		},
		{
			name:     "should return bad request when use case returns 400",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    400,
					Message: "lessonId é obrigatório",
				})
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "lessonId é obrigatório", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:     "should return not found when lesson not found",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Aula não encontrada",
				})
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Aula não encontrada", response["error"])
				assert.Equal(t, "LESSON_NOT_FOUND", response["errorCode"])
			},
		},
		{
			name:     "should return forbidden when AI is not enabled",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    403,
					Message: "IA não está habilitada para este tenant",
				})
			},
			expectedStatus: http.StatusForbidden,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "IA não está habilitada para este tenant", response["error"])
				assert.Equal(t, "AI_NOT_ENABLED", response["errorCode"])
			},
		},
		{
			name:     "should return too many requests when rate limit exceeded",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    429,
					Message: "Muitas requisições. Tente novamente em 5 minutos.",
				})
			},
			expectedStatus: http.StatusTooManyRequests,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Muitas requisições. Tente novamente em 5 minutos.", response["error"])
				assert.Equal(t, "RATE_LIMIT_EXCEEDED", response["errorCode"])
			},
		},
		{
			name:     "should return internal server error when unexpected error occurs",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, errors.New("unexpected error"))
				mockLogger.EXPECT().Error("Unexpected error: unexpected error")
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
				assert.Equal(t, "Erro interno do servidor", response["message"])
			},
		},
		{
			name:     "should return error with custom code when member class error has custom code",
			method:   http.MethodPatch,
			lessonID: "lesson-123",
			apiKey:   "test-api-key",
			requestBody: map[string]bool{
				"transcriptionCompleted": true,
			},
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().UpdateTranscriptionStatus(
					mock.Anything,
					"lesson-123",
					lesson.UpdateLessonTranscriptionRequest{TranscriptionCompleted: true},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    500,
					Message: "Internal error",
				})
			},
			expectedStatus: 500,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
				assert.Equal(t, "Internal error", response["message"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewAILessonHandler(mockUseCase, mockLogger)

			var bodyBytes []byte
			if tt.requestBody != nil {
				if bodyStr, ok := tt.requestBody.(string); ok {
					bodyBytes = []byte(bodyStr)
				} else {
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}

			req := httptest.NewRequest(tt.method, "/api/v1/ai/lessons/"+tt.lessonID, bytes.NewBuffer(bodyBytes))
			req.Header.Set("x-internal-api-key", tt.apiKey)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("lessonId", tt.lessonID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			handler.UpdateTranscriptionStatus(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestAILessonHandler_GetLessons(t *testing.T) {
	os.Setenv("INTERNAL_AI_API_KEY", "test-api-key")
	defer os.Unsetenv("INTERNAL_AI_API_KEY")

	lessonType := "video"
	mediaURL := "https://example.com/video.mp4"
	thumbnail := "https://example.com/thumb.jpg"
	content := "Test content"

	tests := []struct {
		name             string
		method           string
		tenantID         string
		onlyUnprocessed  string
		apiKey           string
		mockSetup        func(*mocks.MockAILessonUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:     "should return method not allowed for POST",
			method:   http.MethodPost,
			tenantID: "tenant-123",
			apiKey:   "test-api-key",
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusMethodNotAllowed,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Method Not Allowed", response["error"])
				assert.Equal(t, "Method not allowed", response["message"])
			},
		},
		{
			name:     "should return unauthorized when api key is missing",
			method:   http.MethodGet,
			tenantID: "tenant-123",
			apiKey:   "",
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:     "should return unauthorized when api key is invalid",
			method:   http.MethodGet,
			tenantID: "tenant-123",
			apiKey:   "wrong-key",
			mockSetup: func(*mocks.MockAILessonUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Não autorizado: token é obrigatório", response["error"])
				assert.Equal(t, "UNAUTHORIZED", response["errorCode"])
			},
		},
		{
			name:            "should return success with all lessons",
			method:          http.MethodGet,
			tenantID:        "tenant-123",
			onlyUnprocessed: "",
			apiKey:          "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "tenant-123",
						OnlyUnprocessed: false,
					},
				).Return(&ai2.AILessonsResponse{
					Lessons: []ai2.AILessonData{
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
							ModuleName:             "Module 1",
							SectionID:              "section-1",
							SectionName:            "Section 1",
							CourseID:               "course-1",
							CourseName:             "Course 1",
							VitrineID:              "vitrine-1",
							VitrineName:            "Vitrine 1",
						},
					},
					Total:           1,
					TenantID:        "tenant-123",
					OnlyUnprocessed: false,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ai2.AILessonsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "tenant-123", response.TenantID)
				assert.Equal(t, 1, response.Total)
				assert.Len(t, response.Lessons, 1)
				assert.Equal(t, "lesson-1", response.Lessons[0].ID)
			},
		},
		{
			name:            "should return success with only unprocessed lessons",
			method:          http.MethodGet,
			tenantID:        "tenant-123",
			onlyUnprocessed: "true",
			apiKey:          "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "tenant-123",
						OnlyUnprocessed: true,
					},
				).Return(&ai2.AILessonsResponse{
					Lessons: []ai2.AILessonData{
						{
							ID:                     "lesson-2",
							Name:                   "Lesson 2",
							Slug:                   "lesson-2",
							TranscriptionCompleted: false,
							ModuleID:               "module-2",
							ModuleName:             "Module 2",
							SectionID:              "section-2",
							SectionName:            "Section 2",
							CourseID:               "course-2",
							CourseName:             "Course 2",
							VitrineID:              "vitrine-2",
							VitrineName:            "Vitrine 2",
						},
					},
					Total:           1,
					TenantID:        "tenant-123",
					OnlyUnprocessed: true,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ai2.AILessonsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "tenant-123", response.TenantID)
				assert.True(t, response.OnlyUnprocessed)
				assert.Equal(t, 1, response.Total)
			},
		},
		{
			name:     "should return bad request when tenantId is missing",
			method:   http.MethodGet,
			tenantID: "",
			apiKey:   "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "",
						OnlyUnprocessed: false,
					},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    400,
					Message: "tenantId é obrigatório",
				})
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "tenantId é obrigatório", response["error"])
				assert.Equal(t, "INVALID_REQUEST", response["errorCode"])
			},
		},
		{
			name:     "should return not found when tenant not found",
			method:   http.MethodGet,
			tenantID: "tenant-123",
			apiKey:   "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "tenant-123",
						OnlyUnprocessed: false,
					},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Tenant não encontrado",
				})
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Tenant não encontrado", response["error"])
			},
		},
		{
			name:     "should return forbidden when AI is not enabled",
			method:   http.MethodGet,
			tenantID: "tenant-123",
			apiKey:   "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "tenant-123",
						OnlyUnprocessed: false,
					},
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    403,
					Message: "IA não está habilitada para este tenant",
				})
			},
			expectedStatus: http.StatusForbidden,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "IA não está habilitada para este tenant", response["error"])
				assert.Equal(t, "AI_NOT_ENABLED", response["errorCode"])
			},
		},
		{
			name:     "should return internal server error when unexpected error occurs",
			method:   http.MethodGet,
			tenantID: "tenant-123",
			apiKey:   "test-api-key",
			mockSetup: func(mockUseCase *mocks.MockAILessonUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLessons(
					mock.Anything,
					ai.GetAILessonsRequest{
						TenantID:        "tenant-123",
						OnlyUnprocessed: false,
					},
				).Return(nil, errors.New("unexpected error"))
				mockLogger.EXPECT().Error("Unexpected error: unexpected error")
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
				assert.Equal(t, "Erro interno do servidor", response["message"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockAILessonUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewAILessonHandler(mockUseCase, mockLogger)

			url := "/api/v1/ai/lessons"
			if tt.tenantID != "" {
				url += "?tenantId=" + tt.tenantID
			}
			if tt.onlyUnprocessed != "" {
				url += "&onlyUnprocessed=" + tt.onlyUnprocessed
			}

			req := httptest.NewRequest(tt.method, url, nil)
			req.Header.Set("x-internal-api-key", tt.apiKey)

			w := httptest.NewRecorder()

			handler.GetLessons(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}
