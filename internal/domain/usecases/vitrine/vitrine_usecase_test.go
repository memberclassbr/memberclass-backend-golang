package vitrine

import (
	"context"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewVitrineUseCase(t *testing.T) {
	mockRepo := mocks.NewMockVitrineRepository(t)

	useCase := NewVitrineUseCase(mockRepo)

	assert.NotNil(t, useCase)
}

func TestVitrineUseCase_GetVitrines(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		mockSetup      func(*mocks.MockVitrineRepository)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *vitrine.VitrineResponse)
	}{
		{
			name:     "should return error when tenantID is empty",
			tenantID: "",
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "tenantId é obrigatório",
			},
		},
		{
			name:     "should return vitrines successfully",
			tenantID: "tenant-123",
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrinesByTenant(
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
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Vitrines, 1)
				assert.Equal(t, "vitrine-1", result.Vitrines[0].ID)
			},
		},
		{
			name:     "should return error when repository fails",
			tenantID: "tenant-123",
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrinesByTenant(
					mock.Anything,
					"tenant-123",
				).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "should return member class error when repository returns it",
			tenantID: "tenant-123",
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrinesByTenant(
					mock.Anything,
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    500,
					Message: "erro ao buscar catálogo",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar catálogo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockVitrineRepository(t)
			tt.mockSetup(mockRepo)

			useCase := NewVitrineUseCase(mockRepo)

			result, err := useCase.GetVitrines(context.Background(), tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
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

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestVitrineUseCase_GetVitrine(t *testing.T) {
	tests := []struct {
		name            string
		vitrineID       string
		tenantID        string
		includeChildren bool
		ctx             context.Context
		mockSetup       func(*mocks.MockVitrineRepository)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.VitrineDetailResponse)
	}{
		{
			name:      "should return error when vitrineID is empty",
			vitrineID: "",
			tenantID:  "tenant-123",
			ctx:       context.Background(),
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "vitrineId é obrigatório",
			},
		},
		{
			name:      "should get tenantID from context when empty",
			vitrineID: "vitrine-123",
			tenantID:  "",
			ctx:       context.WithValue(context.Background(), constants.TenantContextKey, &tenant.Tenant{ID: "tenant-123"}),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrineByID(
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
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "vitrine-123", result.Vitrine.ID)
			},
		},
		{
			name:      "should return error when tenant not in context and tenantID is empty",
			vitrineID: "vitrine-123",
			tenantID:  "",
			ctx:       context.Background(),
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    401,
				Message: "Token de API inválido",
			},
		},
		{
			name:      "should return vitrine successfully",
			vitrineID: "vitrine-123",
			tenantID:  "tenant-123",
			ctx:       context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrineByID(
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
			includeChildren: true,
			expectError:     false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "vitrine-123", result.Vitrine.ID)
			},
		},
		{
			name:      "should return error when repository fails",
			vitrineID: "vitrine-123",
			tenantID:  "tenant-123",
			ctx:       context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetVitrineByID(
					mock.Anything,
					"vitrine-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Vitrine não encontrada",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Vitrine não encontrada",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockVitrineRepository(t)
			tt.mockSetup(mockRepo)

			useCase := NewVitrineUseCase(mockRepo)

			result, err := useCase.GetVitrine(tt.ctx, tt.vitrineID, tt.tenantID, tt.includeChildren)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
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

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestVitrineUseCase_GetCourse(t *testing.T) {
	tests := []struct {
		name            string
		courseID        string
		tenantID        string
		includeChildren bool
		ctx             context.Context
		mockSetup       func(*mocks.MockVitrineRepository)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.CourseDetailResponse)
	}{
		{
			name:     "should return error when courseID is empty",
			courseID: "",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "courseId é obrigatório",
			},
		},
		{
			name:     "should get tenantID from context when empty",
			courseID: "course-123",
			tenantID: "",
			ctx:      context.WithValue(context.Background(), constants.TenantContextKey, &tenant.Tenant{ID: "tenant-123"}),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetCourseByID(
					mock.Anything,
					"course-123",
					"tenant-123",
					false,
				).Return(&vitrine.CourseDetailResponse{
					Course: vitrine.CourseData{
						ID:   "course-123",
						Name: "Course 1",
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "course-123", result.Course.ID)
			},
		},
		{
			name:     "should return course successfully",
			courseID: "course-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetCourseByID(
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
			includeChildren: true,
			expectError:     false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "course-123", result.Course.ID)
			},
		},
		{
			name:     "should return error when repository fails",
			courseID: "course-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetCourseByID(
					mock.Anything,
					"course-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Curso não encontrado",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Curso não encontrado",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockVitrineRepository(t)
			tt.mockSetup(mockRepo)

			useCase := NewVitrineUseCase(mockRepo)

			result, err := useCase.GetCourse(tt.ctx, tt.courseID, tt.tenantID, tt.includeChildren)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
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

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestVitrineUseCase_GetModule(t *testing.T) {
	tests := []struct {
		name            string
		moduleID        string
		tenantID        string
		includeChildren bool
		ctx             context.Context
		mockSetup       func(*mocks.MockVitrineRepository)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.ModuleDetailResponse)
	}{
		{
			name:     "should return error when moduleID is empty",
			moduleID: "",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "moduleId é obrigatório",
			},
		},
		{
			name:     "should get tenantID from context when empty",
			moduleID: "module-123",
			tenantID: "",
			ctx:      context.WithValue(context.Background(), constants.TenantContextKey, &tenant.Tenant{ID: "tenant-123"}),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetModuleByID(
					mock.Anything,
					"module-123",
					"tenant-123",
					false,
				).Return(&vitrine.ModuleDetailResponse{
					Module: vitrine.ModuleData{
						ID:   "module-123",
						Name: "Module 1",
					},
				}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "module-123", result.Module.ID)
			},
		},
		{
			name:     "should return module successfully",
			moduleID: "module-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetModuleByID(
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
			includeChildren: true,
			expectError:     false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "module-123", result.Module.ID)
			},
		},
		{
			name:     "should return error when repository fails",
			moduleID: "module-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetModuleByID(
					mock.Anything,
					"module-123",
					"tenant-123",
					false,
				).Return(nil, &memberclasserrors.MemberClassError{
					Code:    404,
					Message: "Módulo não encontrado",
				})
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Módulo não encontrado",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockVitrineRepository(t)
			tt.mockSetup(mockRepo)

			useCase := NewVitrineUseCase(mockRepo)

			result, err := useCase.GetModule(tt.ctx, tt.moduleID, tt.tenantID, tt.includeChildren)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
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

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestVitrineUseCase_GetLesson(t *testing.T) {
	tests := []struct {
		name           string
		lessonID       string
		tenantID       string
		ctx            context.Context
		mockSetup      func(*mocks.MockVitrineRepository)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *vitrine.LessonDetailResponse)
	}{
		{
			name:     "should return error when lessonID is empty",
			lessonID: "",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(*mocks.MockVitrineRepository) {
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    400,
				Message: "lessonId é obrigatório",
			},
		},
		{
			name:     "should get tenantID from context when empty",
			lessonID: "lesson-123",
			tenantID: "",
			ctx:      context.WithValue(context.Background(), constants.TenantContextKey, &tenant.Tenant{ID: "tenant-123"}),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetLessonByID(
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
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.LessonDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "lesson-123", result.Lesson.ID)
			},
		},
		{
			name:     "should return lesson successfully",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetLessonByID(
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
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.LessonDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "lesson-123", result.Lesson.ID)
			},
		},
		{
			name:     "should return error when repository fails",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			ctx:      context.Background(),
			mockSetup: func(mockRepo *mocks.MockVitrineRepository) {
				mockRepo.EXPECT().GetLessonByID(
					mock.Anything,
					"lesson-123",
					"tenant-123",
				).Return(nil, &memberclasserrors.MemberClassError{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockVitrineRepository(t)
			tt.mockSetup(mockRepo)

			useCase := NewVitrineUseCase(mockRepo)

			result, err := useCase.GetLesson(tt.ctx, tt.lessonID, tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					var memberClassErr *memberclasserrors.MemberClassError
					if errors.As(err, &memberClassErr) {
						assert.Equal(t, tt.expectedError.Code, memberClassErr.Code)
						assert.Equal(t, tt.expectedError.Message, memberClassErr.Message)
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

			mockRepo.AssertExpectations(t)
		})
	}
}
