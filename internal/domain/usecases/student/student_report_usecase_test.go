package student

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	student2 "github.com/memberclass-backend-golang/internal/domain/dto/response/student"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewStudentReportUseCase(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	assert.NotNil(t, useCase)
}

func TestStudentReportUseCase_GetStudentReport_Success_WithCache(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	cachedResponse := student2.StudentReportResponse{
		Alunos: []student2.StudentReport{
			{
				AlunoIDMemberClass: "user-123",
				Email:              "aluno@example.com",
				Cpf:                "12345678900",
			},
		},
		Pagination: dto.PaginationMeta{
			Page:       1,
			Limit:      10,
			TotalCount: 1,
			TotalPages: 1,
		},
	}

	cachedJSON, _ := json.Marshal(cachedResponse)

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(string(cachedJSON), nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Alunos))

	mockCache.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_Success_WithoutCache(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	students := []student2.StudentReport{
		{
			AlunoIDMemberClass:        "user-123",
			Email:                     "aluno@example.com",
			Cpf:                       "12345678900",
			DataCadastro:              time.Now().Format(time.RFC3339),
			EntregasVinculadas:        []string{"Entrega 1"},
			QuantidadeAulasAssistidas: 2,
			AulasAssistidas: []student2.LessonWatched{
				{
					AulaID:        "lesson-1",
					Titulo:        "Aula 1",
					DataAssistida: time.Now().Format(time.RFC3339),
				},
			},
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return(students, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Alunos))
	assert.Equal(t, int64(1), result.Pagination.TotalCount)
	assert.Equal(t, 1, result.Pagination.TotalPages)

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_WithDateFilters(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	req := student.GetStudentReportRequest{
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	students := []student2.StudentReport{
		{
			AlunoIDMemberClass: "user-123",
			Email:              "aluno@example.com",
			Cpf:                "12345678900",
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, &startDate, &endDate, 1, 10).Return(students, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Alunos))

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_ValidationError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  0,
		Limit: 10,
	}

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "page")
}

func TestStudentReportUseCase_GetStudentReport_RepositoryError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return(nil, int64(0), errors.New("database error"))

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "database error", err.Error())

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_Pagination(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  2,
		Limit: 10,
	}

	students := []student2.StudentReport{
		{AlunoIDMemberClass: "user-1", Email: "user1@example.com"},
		{AlunoIDMemberClass: "user-2", Email: "user2@example.com"},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 2, 10).Return(students, int64(25), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Alunos))
	assert.Equal(t, 2, result.Pagination.Page)
	assert.Equal(t, int64(25), result.Pagination.TotalCount)
	assert.Equal(t, 3, result.Pagination.TotalPages)
	assert.True(t, result.Pagination.HasNextPage)
	assert.True(t, result.Pagination.HasPrevPage)

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_EmptyResult(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return([]student2.StudentReport{}, int64(0), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Alunos))
	assert.Equal(t, int64(0), result.Pagination.TotalCount)

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_CacheSetError(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	students := []student2.StudentReport{
		{
			AlunoIDMemberClass: "user-123",
			Email:              "aluno@example.com",
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return(students, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(errors.New("cache set error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Alunos))

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_InvalidCacheJSON(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	students := []student2.StudentReport{
		{
			AlunoIDMemberClass: "user-123",
			Email:              "aluno@example.com",
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("invalid json", nil)
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return(students, int64(1), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Alunos))

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportUseCase_GetStudentReport_InvalidLimit(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 101,
	}

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "limit")
}

func TestStudentReportUseCase_GetStudentReport_EndDateWithoutStartDate(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	endDate := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	req := student.GetStudentReportRequest{
		Page:      1,
		Limit:     10,
		StartDate: nil,
		EndDate:   &endDate,
	}

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "data de início")
}

func TestStudentReportUseCase_GetStudentReport_WithMultipleStudentsAndData(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	mockRepo := mocks.NewMockStudentReportRepository(t)
	mockCache := mocks.NewMockCache(t)

	useCase := NewStudentReportUseCase(mockLogger, mockRepo, mockCache)

	tenantID := "tenant-123"

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	lastAccess := time.Now().Format(time.RFC3339)
	students := []student2.StudentReport{
		{
			AlunoIDMemberClass:        "user-1",
			Email:                     "user1@example.com",
			Cpf:                       "11111111111",
			EntregasVinculadas:        []string{"Entrega 1", "Entrega 2"},
			UltimoAcesso:              &lastAccess,
			QuantidadeAulasAssistidas: 3,
			AulasAssistidas: []student2.LessonWatched{
				{AulaID: "lesson-1", Titulo: "Aula 1", DataAssistida: time.Now().Format(time.RFC3339)},
				{AulaID: "lesson-2", Titulo: "Aula 2", DataAssistida: time.Now().Format(time.RFC3339)},
				{AulaID: "lesson-3", Titulo: "Aula 3", DataAssistida: time.Now().Format(time.RFC3339)},
			},
		},
		{
			AlunoIDMemberClass:        "user-2",
			Email:                     "user2@example.com",
			Cpf:                       "22222222222",
			EntregasVinculadas:        []string{},
			UltimoAcesso:              nil,
			QuantidadeAulasAssistidas: 0,
			AulasAssistidas:           []student2.LessonWatched{},
		},
	}

	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return("", errors.New("cache miss"))
	mockRepo.On("GetStudentsReport", mock.Anything, tenantID, (*time.Time)(nil), (*time.Time)(nil), 1, 10).Return(students, int64(2), nil)
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil)
	mockLogger.On("Debug", mock.AnythingOfType("string")).Return()

	result, err := useCase.GetStudentReport(context.Background(), req, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Alunos))
	assert.Equal(t, "user-1", result.Alunos[0].AlunoIDMemberClass)
	assert.Equal(t, 3, result.Alunos[0].QuantidadeAulasAssistidas)
	assert.Equal(t, 0, result.Alunos[1].QuantidadeAulasAssistidas)
	assert.Nil(t, result.Alunos[1].UltimoAcesso)

	mockCache.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}
