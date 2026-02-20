package vitrine

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/vitrine"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewVitrineRepository(t *testing.T) {
	db, _, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mockLogger := mocks.NewMockLogger(t)
	repository := NewVitrineRepository(db, mockLogger)

	assert.NotNil(t, repository)
}

func TestVitrineRepository_GetVitrinesByTenant(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		mockSetup      func(sqlmock.Sqlmock)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *vitrine.VitrineResponse)
	}{
		{
			name:     "should return empty vitrines when no vitrines found",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 0, result.Total)
				assert.Len(t, result.Vitrines, 0)
			},
		},
		{
			name:     "should return vitrines successfully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Vitrines, 1)
				assert.Equal(t, "vitrine-1", result.Vitrines[0].ID)
				assert.True(t, result.Vitrines[0].Published)
			},
		},
		{
			name:     "should return error when querying vitrines fails",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar catálogo",
			},
		},
		{
			name:     "should return error when querying courses fails",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar cursos",
			},
		},
		{
			name:     "should handle vitrine with order null",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Vitrines[0].Order)
			},
		},
		{
			name:     "should return error when querying sections fails",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar seções",
			},
		},
		{
			name:     "should return error when querying modules fails",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar módulos",
			},
		},
		{
			name:     "should return error when querying lessons fails",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar aulas",
			},
		},
		{
			name:     "should return complete hierarchy successfully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow("course-1", "Course 1", true, 1, "vitrine-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"}).
					AddRow("section-1", "Section 1", 1, "course-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"}).
					AddRow("module-1", "Module 1", true, 1, "section-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"}).
					AddRow("lesson-1", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", 1, "module-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Vitrines, 1)
				assert.Len(t, result.Vitrines[0].Courses, 1)
				assert.Len(t, result.Vitrines[0].Courses[0].Sections, 1)
				assert.Len(t, result.Vitrines[0].Courses[0].Sections[0].Modules, 1)
				assert.Len(t, result.Vitrines[0].Courses[0].Sections[0].Modules[0].Lessons, 1)
			},
		},
		{
			name:     "should handle scan errors gracefully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1).
					AddRow(nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, 1, result.Total)
			},
		},
		{
			name:     "should handle courses with null order",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow("course-1", "Course 1", true, nil, "vitrine-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Vitrines[0].Courses[0].Order)
			},
		},
		{
			name:     "should handle modules with null order",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow("course-1", "Course 1", true, 1, "vitrine-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"}).
					AddRow("section-1", "Section 1", 1, "course-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"}).
					AddRow("module-1", "Module 1", true, nil, "section-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Vitrines[0].Courses[0].Sections[0].Modules[0].Order)
			},
		},
		{
			name:     "should handle scan errors in courses gracefully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow(nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrines[0].Courses, 0)
			},
		},
		{
			name:     "should handle scan errors in modules gracefully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow("course-1", "Course 1", true, 1, "vitrine-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"}).
					AddRow("section-1", "Section 1", 1, "course-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"}).
					AddRow(nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrines[0].Courses[0].Sections[0].Modules, 0)
			},
		},
		{
			name:     "should handle scan errors in lessons gracefully",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				vitrinesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-1", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("tenant-123").
					WillReturnRows(vitrinesRows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "vitrineId"}).
					AddRow("course-1", "Course 1", true, 1, "vitrine-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("tenant-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order", "courseId"}).
					AddRow("section-1", "Section 1", 1, "course-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("tenant-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order", "sectionId"}).
					AddRow("module-1", "Module 1", true, 1, "section-1")
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("tenant-123").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order", "moduleId"}).
					AddRow(nil, nil, nil, nil, nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("tenant-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrines[0].Courses[0].Sections[0].Modules[0].Lessons, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}
			if tt.name == "should handle scan errors gracefully" ||
				tt.name == "should handle scan errors in courses gracefully" ||
				tt.name == "should handle scan errors in modules gracefully" ||
				tt.name == "should handle scan errors in lessons gracefully" {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewVitrineRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.GetVitrinesByTenant(context.Background(), tt.tenantID)

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

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestVitrineRepository_GetVitrineByID(t *testing.T) {
	tests := []struct {
		name            string
		vitrineID       string
		tenantID        string
		includeChildren bool
		mockSetup       func(sqlmock.Sqlmock)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.VitrineDetailResponse)
	}{
		{
			name:            "should return vitrine without children successfully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "vitrine-123", result.Vitrine.ID)
				assert.True(t, result.Vitrine.Published)
				assert.Len(t, result.Vitrine.Courses, 0)
			},
		},
		{
			name:            "should return vitrine with children successfully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"})
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "vitrine-123", result.Vitrine.ID)
			},
		},
		{
			name:            "should return vitrine with complete hierarchy",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-1", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "vitrine-123", result.Vitrine.ID)
				assert.Len(t, result.Vitrine.Courses, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections[0].Modules, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections[0].Modules[0].Lessons, 1)
			},
		},
		{
			name:            "should handle vitrine with null order",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Vitrine.Order)
			},
		},
		{
			name:            "should handle scan errors in courses gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow(nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses, 0)
			},
		},
		{
			name:            "should handle scan errors in modules gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow(nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections[0].Modules, 0)
			},
		},
		{
			name:            "should handle scan errors in lessons gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow(nil, nil, nil, nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses[0].Sections[0].Modules[0].Lessons, 0)
			},
		},
		{
			name:            "should handle query errors in sections gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnError(errors.New("database error"))
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses, 0)
			},
		},
		{
			name:            "should handle query errors in modules gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnRows(sectionsRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnError(errors.New("database error"))
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses, 1)
				assert.Len(t, result.Vitrine.Courses[0].Sections, 0)
			},
		},
		{
			name:            "should handle query errors in lessons gracefully",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				coursesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-1", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnRows(coursesRows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-1").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnError(errors.New("database error"))
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.VitrineDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Vitrine.Courses, 1)
				if len(result.Vitrine.Courses) > 0 {
					assert.Len(t, result.Vitrine.Courses[0].Sections, 1)
					if len(result.Vitrine.Courses[0].Sections) > 0 {
						assert.Len(t, result.Vitrine.Courses[0].Sections[0].Modules, 0)
					}
				}
			},
		},
		{
			name:      "should return error when vitrine not found",
			vitrineID: "vitrine-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Vitrine não encontrada",
			},
		},
		{
			name:      "should return error when database error occurs",
			vitrineID: "vitrine-123",
			tenantID:  "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar vitrine",
			},
		},
		{
			name:            "should return error when querying courses fails",
			vitrineID:       "vitrine-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("vitrine-123", "Vitrine 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Vitrine"`).
					WithArgs("vitrine-123", "tenant-123").
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("vitrine-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar cursos",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError && tt.expectedError != nil && tt.expectedError.Code == 500 {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}
			if tt.name == "should handle scan errors in courses gracefully" ||
				tt.name == "should handle scan errors in modules gracefully" ||
				tt.name == "should handle scan errors in lessons gracefully" ||
				tt.name == "should handle query errors in sections gracefully" ||
				tt.name == "should handle query errors in modules gracefully" ||
				tt.name == "should handle query errors in lessons gracefully" {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewVitrineRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.GetVitrineByID(context.Background(), tt.vitrineID, tt.tenantID, tt.includeChildren)

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

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestVitrineRepository_GetCourseByID(t *testing.T) {
	tests := []struct {
		name            string
		courseID        string
		tenantID        string
		includeChildren bool
		mockSetup       func(sqlmock.Sqlmock)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.CourseDetailResponse)
	}{
		{
			name:            "should return course without children successfully",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "course-123", result.Course.ID)
				assert.True(t, result.Course.Published)
				assert.Len(t, result.Course.Sections, 0)
			},
		},
		{
			name:            "should return course with children successfully",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-1", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "course-123", result.Course.ID)
				assert.Len(t, result.Course.Sections, 1)
				assert.Len(t, result.Course.Sections[0].Modules, 1)
				assert.Len(t, result.Course.Sections[0].Modules[0].Lessons, 1)
			},
		},
		{
			name:            "should handle course with null order",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Course.Order)
			},
		},
		{
			name:            "should handle scan errors in modules gracefully for course",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow(nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Course.Sections, 1)
				assert.Len(t, result.Course.Sections[0].Modules, 0)
			},
		},
		{
			name:            "should handle scan errors in lessons gracefully for course",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow(nil, nil, nil, nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Course.Sections[0].Modules[0].Lessons, 0)
			},
		},
		{
			name:            "should handle query errors in lessons gracefully for course",
			courseID:        "course-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("course-123", "Course 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnRows(rows)

				sectionsRows := sqlmock.NewRows([]string{"id", "name", "order"}).
					AddRow("section-1", "Section 1", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Section"`).
					WithArgs("course-123").
					WillReturnRows(sectionsRows)

				modulesRows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-1", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("section-1").
					WillReturnRows(modulesRows)

				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-1").
					WillReturnError(errors.New("database error"))
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.CourseDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Course.Sections, 1)
				assert.Len(t, result.Course.Sections[0].Modules, 0)
			},
		},
		{
			name:     "should return error when course not found",
			courseID: "course-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Curso não encontrado",
			},
		},
		{
			name:     "should return error when database error occurs",
			courseID: "course-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Course"`).
					WithArgs("course-123", "tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar curso",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError && tt.expectedError != nil && tt.expectedError.Code == 500 {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}
			if tt.name == "should handle scan errors in modules gracefully for course" ||
				tt.name == "should handle scan errors in lessons gracefully for course" ||
				tt.name == "should handle query errors in lessons gracefully for course" {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewVitrineRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.GetCourseByID(context.Background(), tt.courseID, tt.tenantID, tt.includeChildren)

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

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestVitrineRepository_GetModuleByID(t *testing.T) {
	tests := []struct {
		name            string
		moduleID        string
		tenantID        string
		includeChildren bool
		mockSetup       func(sqlmock.Sqlmock)
		expectError     bool
		expectedError   *memberclasserrors.MemberClassError
		validateResult  func(*testing.T, *vitrine.ModuleDetailResponse)
	}{
		{
			name:            "should return module without children successfully",
			moduleID:        "module-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-123", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "module-123", result.Module.ID)
				assert.True(t, result.Module.Published)
				assert.Len(t, result.Module.Lessons, 0)
			},
		},
		{
			name:            "should return module with children successfully",
			moduleID:        "module-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-123", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnRows(rows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-1", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "module-123", result.Module.ID)
				assert.Len(t, result.Module.Lessons, 1)
				assert.True(t, result.Module.Lessons[0].Published)
			},
		},
		{
			name:            "should handle module with null order",
			moduleID:        "module-123",
			tenantID:        "tenant-123",
			includeChildren: false,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-123", "Module 1", true, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Module.Order)
			},
		},
		{
			name:            "should handle scan errors in lessons gracefully for module",
			moduleID:        "module-123",
			tenantID:        "tenant-123",
			includeChildren: true,
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "order"}).
					AddRow("module-123", "Module 1", true, 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnRows(rows)

				lessonsRows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow(nil, nil, nil, nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("module-123").
					WillReturnRows(lessonsRows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.ModuleDetailResponse) {
				assert.NotNil(t, result)
				assert.Len(t, result.Module.Lessons, 0)
			},
		},
		{
			name:     "should return error when module not found",
			moduleID: "module-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Módulo não encontrado",
			},
		},
		{
			name:     "should return error when database error occurs",
			moduleID: "module-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Module"`).
					WithArgs("module-123", "tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar módulo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError && tt.expectedError != nil && tt.expectedError.Code == 500 {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}
			if tt.name == "should handle scan errors in lessons gracefully for module" {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewVitrineRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.GetModuleByID(context.Background(), tt.moduleID, tt.tenantID, tt.includeChildren)

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

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestVitrineRepository_GetLessonByID(t *testing.T) {
	tests := []struct {
		name           string
		lessonID       string
		tenantID       string
		mockSetup      func(sqlmock.Sqlmock)
		expectError    bool
		expectedError  *memberclasserrors.MemberClassError
		validateResult func(*testing.T, *vitrine.LessonDetailResponse)
	}{
		{
			name:     "should return lesson successfully",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-123", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", 1)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("lesson-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.LessonDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "lesson-123", result.Lesson.ID)
				assert.True(t, result.Lesson.Published)
				assert.NotNil(t, result.Lesson.Slug)
				assert.Equal(t, "lesson-1", *result.Lesson.Slug)
			},
		},
		{
			name:     "should return lesson with null fields",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-123", "Lesson 1", true, nil, nil, nil, nil, nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("lesson-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.LessonDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "lesson-123", result.Lesson.ID)
				assert.Nil(t, result.Lesson.Slug)
				assert.Nil(t, result.Lesson.Type)
			},
		},
		{
			name:     "should return lesson with null order",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "published", "slug", "type", "mediaUrl", "thumbnail", "order"}).
					AddRow("lesson-123", "Lesson 1", true, "lesson-1", "video", "https://example.com/video.mp4", "https://example.com/thumb.jpg", nil)
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("lesson-123", "tenant-123").
					WillReturnRows(rows)
			},
			expectError: false,
			validateResult: func(t *testing.T, result *vitrine.LessonDetailResponse) {
				assert.NotNil(t, result)
				assert.Equal(t, "lesson-123", result.Lesson.ID)
				assert.Nil(t, result.Lesson.Order)
			},
		},
		{
			name:     "should return error when lesson not found",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("lesson-123", "tenant-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Aula não encontrada",
			},
		},
		{
			name:     "should return error when database error occurs",
			lessonID: "lesson-123",
			tenantID: "tenant-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT.*FROM "Lesson"`).
					WithArgs("lesson-123", "tenant-123").
					WillReturnError(errors.New("database error"))
			},
			expectError: true,
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar aula",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectError && tt.expectedError != nil && tt.expectedError.Code == 500 {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewVitrineRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.GetLessonByID(context.Background(), tt.lessonID, tt.tenantID)

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

			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}
