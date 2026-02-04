package student

import (
	"context"
	"time"

	studentreq "github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	studentresp "github.com/memberclass-backend-golang/internal/domain/dto/response/student"
)

type StudentReportRepository interface {
	GetStudentsReport(ctx context.Context, tenantID string, startDate, endDate *time.Time, page, limit int) ([]studentresp.StudentReport, int64, error)
	GetStudentsRanking(ctx context.Context, req studentreq.GetStudentsRankingRequest, start, end time.Time) ([]studentresp.StudentRankingRow, int64, error)
}
