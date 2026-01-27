package student

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/constants"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	student2 "github.com/memberclass-backend-golang/internal/domain/dto/response/student"
	"github.com/memberclass-backend-golang/internal/domain/entities/tenant"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewStudentReportHandler(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, mockUseCase, handler.useCase)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestStudentReportHandler_GetStudentReport_Success(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	students := []student2.StudentReport{
		{
			AlunoIDMemberClass:        "user-123",
			Email:                     "aluno@example.com",
			Cpf:                       "12345678900",
			DataCadastro:              time.Now().Format(time.RFC3339),
			EntregasVinculadas:        []string{"Entrega 1"},
			QuantidadeAulasAssistidas: 1,
			AulasAssistidas: []student2.LessonWatched{
				{
					AulaID:        "lesson-1",
					Titulo:        "Aula 1",
					DataAssistida: time.Now().Format(time.RFC3339),
				},
			},
		},
	}

	reportResponse := &student2.StudentReportResponse{
		Alunos: students,
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  1,
			TotalPages:  1,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetStudentReport", mock.Anything, req, tenantID).Return(reportResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetStudentReport(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result student2.StudentReportResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Alunos))
	assert.Equal(t, "user-123", result.Alunos[0].AlunoIDMemberClass)
	assert.Equal(t, "aluno@example.com", result.Alunos[0].Email)

	mockUseCase.AssertExpectations(t)
}

func TestStudentReportHandler_GetStudentReport_MethodNotAllowed(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("POST", "/api/v1/student/report", nil)
	w := httptest.NewRecorder()

	handler.GetStudentReport(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestStudentReportHandler_GetStudentReport_InvalidPagination(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/student/report?page=0&limit=10", nil)
	w := httptest.NewRecorder()

	handler.GetStudentReport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Parâmetros de paginação inválidos. page >= 1, limit entre 1 e 100", response["error"])
	assert.Equal(t, "INVALID_PAGINATION", response["errorCode"])
}

func TestStudentReportHandler_GetStudentReport_InvalidDateFormat(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10&startDate=2024-01-01", nil)
	w := httptest.NewRecorder()

	handler.GetStudentReport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Contains(t, response["error"].(string), "formato de data inválido")
	assert.Equal(t, "INVALID_DATE_FORMAT", response["errorCode"])
}

func TestStudentReportHandler_GetStudentReport_InvalidDateRange(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10&startDate=2024-12-31T23:59:59Z&endDate=2024-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()

	handler.GetStudentReport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "startDate não pode ser maior que endDate", response["error"])
	assert.Equal(t, "INVALID_DATE_RANGE", response["errorCode"])
}

func TestStudentReportHandler_GetStudentReport_TenantNotFound(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	req := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10", nil)
	w := httptest.NewRecorder()

	handler.GetStudentReport(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "API key inválida", response["error"])
	assert.Equal(t, "INVALID_API_KEY", response["errorCode"])
}

func TestStudentReportHandler_GetStudentReport_MemberClassError(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	memberClassErr := &memberclasserrors.MemberClassError{
		Code:    http.StatusInternalServerError,
		Message: "Database error",
	}

	mockUseCase.On("GetStudentReport", mock.Anything, req, tenantID).Return(nil, memberClassErr)

	httpReq := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetStudentReport(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Database error", response["error"])
	assert.Equal(t, "INTERNAL_ERROR", response["errorCode"])

	mockUseCase.AssertExpectations(t)
}

func TestStudentReportHandler_GetStudentReport_UnexpectedError(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetStudentReport", mock.Anything, req, tenantID).Return(nil, errors.New("unexpected error"))
	mockLogger.On("Error", mock.AnythingOfType("string")).Return()

	httpReq := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetStudentReport(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response["ok"])
	assert.Equal(t, "Erro interno do servidor", response["error"])
	assert.Equal(t, "INTERNAL_ERROR", response["errorCode"])

	mockUseCase.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

func TestStudentReportHandler_GetStudentReport_EmptyResult(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	reportResponse := &student2.StudentReportResponse{
		Alunos: []student2.StudentReport{},
		Pagination: dto.PaginationMeta{
			Page:        1,
			Limit:       10,
			TotalCount:  0,
			TotalPages:  0,
			HasNextPage: false,
			HasPrevPage: false,
		},
	}

	req := student.GetStudentReportRequest{
		Page:  1,
		Limit: 10,
	}

	mockUseCase.On("GetStudentReport", mock.Anything, req, tenantID).Return(reportResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetStudentReport(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result student2.StudentReportResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Alunos))
	assert.Equal(t, int64(0), result.Pagination.TotalCount)

	mockUseCase.AssertExpectations(t)
}

func TestStudentReportHandler_GetStudentReport_WithDateFilters(t *testing.T) {
	mockUseCase := mocks.NewMockStudentReportUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewStudentReportHandler(mockUseCase, mockLogger)

	tenantID := "tenant-123"
	tenant := &tenant.Tenant{ID: tenantID}

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	reportResponse := &student2.StudentReportResponse{
		Alunos: []student2.StudentReport{
			{
				AlunoIDMemberClass: "user-123",
				Email:              "aluno@example.com",
			},
		},
		Pagination: dto.PaginationMeta{
			Page:       1,
			Limit:      10,
			TotalCount: 1,
			TotalPages: 1,
		},
	}

	req := student.GetStudentReportRequest{
		Page:      1,
		Limit:     10,
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	mockUseCase.On("GetStudentReport", mock.Anything, req, tenantID).Return(reportResponse, nil)

	httpReq := httptest.NewRequest("GET", "/api/v1/student/report?page=1&limit=10&startDate=2024-01-01T00:00:00Z&endDate=2024-12-31T23:59:59Z", nil)
	ctx := context.WithValue(httpReq.Context(), constants.TenantContextKey, tenant)
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetStudentReport(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var result student2.StudentReportResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Alunos))

	mockUseCase.AssertExpectations(t)
}
