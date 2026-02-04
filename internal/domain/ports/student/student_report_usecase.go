package student

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	student2 "github.com/memberclass-backend-golang/internal/domain/dto/response/student"
)

type StudentReportUseCase interface {
	GetStudentReport(ctx context.Context, req student.GetStudentReportRequest, tenantID string) (*student2.StudentReportResponse, error)
	GetStudentsRanking(ctx context.Context, req student.GetStudentsRankingRequest, tenantID string) (*student2.StudentsRankingResponse, bool, error)
}
