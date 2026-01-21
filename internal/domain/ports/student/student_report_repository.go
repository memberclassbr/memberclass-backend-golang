package student

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/student"
)

type StudentReportRepository interface {
	GetStudentsReport(ctx context.Context, tenantID string, startDate, endDate *time.Time, page, limit int) ([]student.StudentReport, int64, error)
}
