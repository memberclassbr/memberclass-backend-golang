package ports

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type StudentReportRepository interface {
	GetStudentsReport(ctx context.Context, tenantID string, startDate, endDate *time.Time, page, limit int) ([]response.StudentReport, int64, error)
}

