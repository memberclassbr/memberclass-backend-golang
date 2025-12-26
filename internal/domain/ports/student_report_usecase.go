package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type StudentReportUseCase interface {
	GetStudentReport(ctx context.Context, req request.GetStudentReportRequest, tenantID string) (*response.StudentReportResponse, error)
}

