package vitrine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewVitrineHandler(t *testing.T) {
	mockUseCase := mocks.NewMockVitrineUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewVitrineHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestVitrineHandler_GetVitrines(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		tenant           *tenant.Tenant
		mockSetup        func(*mocks.MockVitrineUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return method not allowed for POST",
			method: http.MethodPost,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusMethodNotAllowed,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Method Not Allowed", response["error"])
			},
		},
		{
			name:   "should return unauthorized when tenant is nil",
			method: http.MethodGet,
			tenant: nil,
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Token de API inválido", response["error"])
				assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
			},
		},
		{
			name:   "should return vitrines successfully",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrines(
					mock.Anything,
					"tenant-123",
				).Return(&vitrine.VitrineResponse{
					Vitrines: []vitrine.VitrineData{
						{
							ID:   "vitrine-1",
							Name: "Vitrine 1",
						},
					},
					Total: 1,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response vitrine.VitrineResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, 1, response.Total)
				assert.Len(t, response.Vitrines, 1)
			},
		},
		{
			name:   "should return bad request when use case returns validation error",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrines(
					mock.Anything,
					"tenant-123",
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
			},
		},
		{
			name:   "should return not found when vitrines not found",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrines(
					mock.Anything,
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Catálogo não encontrado",
				})
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "Catálogo não encontrado", response["error"])
			},
		},
		{
			name:   "should return internal server error when unexpected error occurs",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrines(
					mock.Anything,
					"tenant-123",
				).Return(nil, errors.New("unexpected error"))
				mockLogger.EXPECT().Error("Unexpected error: unexpected error")
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "Internal Server Error", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewVitrineHandler(mockUseCase, mockLogger)

			req := httptest.NewRequest(tt.method, "/api/v1/vitrine", nil)
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()

			handler.GetVitrines(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestVitrineHandler_GetVitrine(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		vitrineID        string
		includeChildren  string
		tenant           *tenant.Tenant
		mockSetup        func(*mocks.MockVitrineUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return method not allowed for POST",
			method: http.MethodPost,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "should return bad request when vitrineID is empty",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, false, response["ok"])
				assert.Equal(t, "vitrineId é obrigatório", response["error"])
			},
		},
		{
			name:      "should return unauthorized when tenant is nil",
			method:    http.MethodGet,
			vitrineID: "vitrine-123",
			tenant:    nil,
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:            "should return vitrine successfully",
			method:          http.MethodGet,
			vitrineID:       "vitrine-123",
			includeChildren: "true",
			tenant:          &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrine(
					mock.Anything,
					"vitrine-123",
					"tenant-123",
					true,
				).Return(&vitrine.VitrineDetailResponse{
					Vitrine: vitrine.VitrineData{
						ID:   "vitrine-123",
						Name: "Vitrine 1",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response vitrine.VitrineDetailResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "vitrine-123", response.Vitrine.ID)
			},
		},
		{
			name:      "should parse includeChildren as false when not provided",
			method:    http.MethodGet,
			vitrineID: "vitrine-123",
			tenant:    &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrine(
					mock.Anything,
					"vitrine-123",
					"tenant-123",
					false,
				).Return(&vitrine.VitrineDetailResponse{
					Vitrine: vitrine.VitrineData{
						ID:   "vitrine-123",
						Name: "Vitrine 1",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "should return not found when vitrine not found",
			method:    http.MethodGet,
			vitrineID: "vitrine-123",
			tenant:    &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetVitrine(
					mock.Anything,
					"vitrine-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Vitrine não encontrada",
				})
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewVitrineHandler(mockUseCase, mockLogger)

			url := "/api/v1/vitrine/" + tt.vitrineID
			if tt.includeChildren != "" {
				url += "?includeChildren=" + tt.includeChildren
			}

			req := httptest.NewRequest(tt.method, url, nil)
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("vitrineId", tt.vitrineID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			handler.GetVitrine(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestVitrineHandler_GetCourse(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		courseID         string
		includeChildren  string
		tenant           *tenant.Tenant
		mockSetup        func(*mocks.MockVitrineUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return bad request when courseID is empty",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:            "should return course successfully",
			method:          http.MethodGet,
			courseID:        "course-123",
			includeChildren: "true",
			tenant:          &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetCourse(
					mock.Anything,
					"course-123",
					"tenant-123",
					true,
				).Return(&vitrine.CourseDetailResponse{
					Course: vitrine.CourseData{
						ID:   "course-123",
						Name: "Course 1",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response vitrine.CourseDetailResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "course-123", response.Course.ID)
			},
		},
		{
			name:     "should return not found when course not found",
			method:   http.MethodGet,
			courseID: "course-123",
			tenant:   &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetCourse(
					mock.Anything,
					"course-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Curso não encontrado",
				})
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewVitrineHandler(mockUseCase, mockLogger)

			url := "/api/v1/vitrine/courses/" + tt.courseID
			if tt.includeChildren != "" {
				url += "?includeChildren=" + tt.includeChildren
			}

			req := httptest.NewRequest(tt.method, url, nil)
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("courseId", tt.courseID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			handler.GetCourse(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestVitrineHandler_GetModule(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		moduleID         string
		includeChildren  string
		tenant           *tenant.Tenant
		mockSetup        func(*mocks.MockVitrineUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return bad request when moduleID is empty",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:            "should return module successfully",
			method:          http.MethodGet,
			moduleID:        "module-123",
			includeChildren: "true",
			tenant:          &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetModule(
					mock.Anything,
					"module-123",
					"tenant-123",
					true,
				).Return(&vitrine.ModuleDetailResponse{
					Module: vitrine.ModuleData{
						ID:   "module-123",
						Name: "Module 1",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response vitrine.ModuleDetailResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "module-123", response.Module.ID)
			},
		},
		{
			name:     "should return not found when module not found",
			method:   http.MethodGet,
			moduleID: "module-123",
			tenant:   &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetModule(
					mock.Anything,
					"module-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Módulo não encontrado",
				})
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewVitrineHandler(mockUseCase, mockLogger)

			url := "/api/v1/vitrine/modules/" + tt.moduleID
			if tt.includeChildren != "" {
				url += "?includeChildren=" + tt.includeChildren
			}

			req := httptest.NewRequest(tt.method, url, nil)
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("moduleId", tt.moduleID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			handler.GetModule(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestVitrineHandler_GetLesson(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		lessonID         string
		tenant           *tenant.Tenant
		mockSetup        func(*mocks.MockVitrineUseCase, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "should return bad request when lessonID is empty",
			method: http.MethodGet,
			tenant: &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(*mocks.MockVitrineUseCase, *mocks.MockLogger) {
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "should return lesson successfully",
			method:   http.MethodGet,
			lessonID: "lesson-123",
			tenant:   &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLesson(
					mock.Anything,
					"lesson-123",
					"tenant-123",
				).Return(&vitrine.LessonDetailResponse{
					Lesson: vitrine.LessonData{
						ID:   "lesson-123",
						Name: "Lesson 1",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response vitrine.LessonDetailResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "lesson-123", response.Lesson.ID)
			},
		},
		{
			name:     "should return not found when lesson not found",
			method:   http.MethodGet,
			lessonID: "lesson-123",
			tenant:   &tenant.Tenant{ID: "tenant-123"},
			mockSetup: func(mockUseCase *mocks.MockVitrineUseCase, mockLogger *mocks.MockLogger) {
				mockUseCase.EXPECT().GetLesson(
					mock.Anything,
					"lesson-123",
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Aula não encontrada",
				})
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockUseCase, mockLogger)

			handler := NewVitrineHandler(mockUseCase, mockLogger)

			req := httptest.NewRequest(tt.method, "/api/v1/vitrine/lessons/"+tt.lessonID, nil)
			if tt.tenant != nil {
				ctx := context.WithValue(req.Context(), constants.TenantContextKey, tt.tenant)
				req = req.WithContext(ctx)
			}

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("lessonId", tt.lessonID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			handler.GetLesson(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}

			mockUseCase.AssertExpectations(t)
		})
	}
}

func TestVitrineHandler_parseIncludeChildren(t *testing.T) {
	tests := []struct {
		name           string
		queryParam     string
		expectedResult bool
	}{
		{
			name:           "should return true when includeChildren is true",
			queryParam:     "true",
			expectedResult: true,
		},
		{
			name:           "should return false when includeChildren is false",
			queryParam:     "false",
			expectedResult: false,
		},
		{
			name:           "should return false when includeChildren is empty",
			queryParam:     "",
			expectedResult: false,
		},
		{
			name:           "should return false when includeChildren is invalid",
			queryParam:     "invalid",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUseCase := mocks.NewMockVitrineUseCase(t)
			mockLogger := mocks.NewMockLogger(t)
			handler := NewVitrineHandler(mockUseCase, mockLogger)

			url := "/api/v1/vitrine/vitrine-123"
			if tt.queryParam != "" {
				url += "?includeChildren=" + tt.queryParam
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			result := handler.parseIncludeChildren(req)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
