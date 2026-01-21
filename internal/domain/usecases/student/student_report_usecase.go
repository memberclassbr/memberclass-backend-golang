package student

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	student2 "github.com/memberclass-backend-golang/internal/domain/dto/response/student"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	student3 "github.com/memberclass-backend-golang/internal/domain/ports/student"
)

type StudentReportUseCase struct {
	logger            ports.Logger
	studentReportRepo student3.StudentReportRepository
	cache             ports.Cache
}

func NewStudentReportUseCase(logger ports.Logger, studentReportRepo student3.StudentReportRepository, cache ports.Cache) student3.StudentReportUseCase {
	return &StudentReportUseCase{
		logger:            logger,
		studentReportRepo: studentReportRepo,
		cache:             cache,
	}
}

func (uc *StudentReportUseCase) GetStudentReport(ctx context.Context, req student.GetStudentReportRequest, tenantID string) (*student2.StudentReportResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	cacheKey := uc.buildCacheKey(tenantID, req)

	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cachedResponse student2.StudentReportResponse
		if err := json.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
			uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &cachedResponse, nil
		}
	}

	students, totalCount, err := uc.studentReportRepo.GetStudentsReport(ctx, tenantID, req.StartDate, req.EndDate, req.Page, req.Limit)
	if err != nil {
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(req.Limit)))

	pagination := student2.StudentReportPagination{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  int(totalCount),
		TotalPages:  totalPages,
		HasNextPage: req.Page < totalPages,
		HasPrevPage: req.Page > 1,
	}

	responseData := &student2.StudentReportResponse{
		Alunos:     students,
		Pagination: pagination,
	}

	responseJSON, err := json.Marshal(responseData)
	if err == nil {
		cacheExpiration := 300 * time.Second
		if err := uc.cache.Set(ctx, cacheKey, string(responseJSON), cacheExpiration); err != nil {
			uc.logger.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			uc.logger.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return responseData, nil
}

func (uc *StudentReportUseCase) buildCacheKey(tenantID string, req student.GetStudentReportRequest) string {
	cacheData := map[string]interface{}{
		"tenantId":  tenantID,
		"page":      req.Page,
		"limit":     req.Limit,
		"startDate": nil,
		"endDate":   nil,
	}

	if req.StartDate != nil {
		cacheData["startDate"] = req.StartDate.Format(time.RFC3339)
	}

	if req.EndDate != nil {
		cacheData["endDate"] = req.EndDate.Format(time.RFC3339)
	}

	jsonData, _ := json.Marshal(cacheData)
	hash := md5.Sum(jsonData)
	hashStr := hex.EncodeToString(hash[:])

	return fmt.Sprintf("alunos_relatorio:%s", hashStr)
}
