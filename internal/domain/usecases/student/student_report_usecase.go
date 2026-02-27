package student

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
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

	pagination := dto.PaginationMeta{
		Page:        req.Page,
		Limit:       req.Limit,
		TotalCount:  totalCount,
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

func (uc *StudentReportUseCase) GetStudentsRanking(ctx context.Context, req student.GetStudentsRankingRequest, tenantID string) (*student2.StudentsRankingResponse, bool, error) {
	if err := req.Validate(); err != nil {
		return nil, false, err
	}

	start, end := uc.rankingPeriodBounds(req)
	cacheKey := uc.buildRankingCacheKey(tenantID, req, start, end)

	cachedData, err := uc.cache.Get(ctx, cacheKey)
	if err == nil && cachedData != "" {
		var cached student2.StudentsRankingResponse
		if err := json.Unmarshal([]byte(cachedData), &cached); err == nil {
			limit := req.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}
			endIdx := (req.Page-1)*limit + limit
			if len(cached.Rankings) >= endIdx || len(cached.Rankings) == int(cached.TotalStudents) {
				uc.logger.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
				return uc.paginateRankingResponse(&cached, req), true, nil
			}
		}
	}

	reqAll := req
	reqAll.Page = 1
	reqAll.Limit = 0
	rows, total, err := uc.studentReportRepo.GetStudentsRanking(ctx, reqAll, start, end)
	if err != nil {
		return nil, false, err
	}

	rankings := make([]student2.StudentRankingEntry, 0, len(rows))
	for i, row := range rows {
		rankings = append(rankings, student2.StudentRankingEntry{
			Position: i + 1,
			Student: student2.StudentRankingInfo{
				ID:      row.UserID,
				Name:    row.Name,
				Picture: row.Picture,
				Email:   row.Email,
			},
			Metrics: student2.StudentRankingMetrics{
				Logins:         row.Logins,
				LessonsWatched: row.LessonsWatched,
				Comments:       row.Comments,
				Ratings:        row.Ratings,
				AvgRating:      row.AvgRating,
			},
		})
	}

	fullResp := &student2.StudentsRankingResponse{
		Period: student2.PeriodRange{
			Start: start.Format(time.RFC3339),
			End:   end.Format(time.RFC3339),
		},
		TotalStudents: total,
		Rankings:      rankings,
	}

	responseJSON, err := json.Marshal(fullResp)
	if err == nil {
		ttl := 12 * time.Hour
		if err := uc.cache.Set(ctx, cacheKey, string(responseJSON), ttl); err != nil {
			uc.logger.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		}
	}

	return uc.paginateRankingResponse(fullResp, req), false, nil
}

func (uc *StudentReportUseCase) paginateRankingResponse(full *student2.StudentsRankingResponse, req student.GetStudentsRankingRequest) *student2.StudentsRankingResponse {
	total := full.TotalStudents
	limit := req.Limit
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	startIdx := (page - 1) * limit
	endIdx := startIdx + limit
	if startIdx >= len(full.Rankings) {
		rankingsPage := []student2.StudentRankingEntry{}
		totalPages := int(math.Ceil(float64(total) / float64(limit)))
		pagination := dto.PaginationMeta{
			Page:        page,
			Limit:       limit,
			TotalCount:  total,
			TotalPages:  totalPages,
			HasNextPage: page < totalPages,
			HasPrevPage: page > 1,
		}
		return &student2.StudentsRankingResponse{
			Period:        full.Period,
			TotalStudents: total,
			Rankings:      rankingsPage,
			Pagination:    pagination,
		}
	}
	if endIdx > len(full.Rankings) {
		endIdx = len(full.Rankings)
	}
	rankingsPage := make([]student2.StudentRankingEntry, endIdx-startIdx)
	copy(rankingsPage, full.Rankings[startIdx:endIdx])
	for i := range rankingsPage {
		rankingsPage[i].Position = startIdx + i + 1
	}
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	pagination := dto.PaginationMeta{
		Page:        page,
		Limit:       limit,
		TotalCount:  total,
		TotalPages:  totalPages,
		HasNextPage: page < totalPages,
		HasPrevPage: page > 1,
	}
	return &student2.StudentsRankingResponse{
		Period:        full.Period,
		TotalStudents: total,
		Rankings:      rankingsPage,
		Pagination:    pagination,
	}
}

func (uc *StudentReportUseCase) rankingPeriodBounds(req student.GetStudentsRankingRequest) (start, end time.Time) {
	end = time.Now()
	switch req.Period {
	case student.PeriodWeek:
		start = end.AddDate(0, 0, -7)
	case student.PeriodMonth:
		start = end.AddDate(0, 0, -30)
	default:
		start = *req.StartDate
		end = *req.EndDate
	}
	return start, end
}

func (uc *StudentReportUseCase) buildRankingCacheKey(tenantID string, req student.GetStudentsRankingRequest, start, end time.Time) string {
	startKey := start.Format(time.RFC3339)
	endKey := end.Format(time.RFC3339)
	if req.Period == student.PeriodWeek || req.Period == student.PeriodMonth {
		endTrunc := end.Truncate(24 * time.Hour)
		endKey = endTrunc.Format("2006-01-02")
		if req.Period == student.PeriodMonth {
			endKey = endTrunc.Format("2006-01")
		}
		startKey = endKey
	}
	cacheData := map[string]interface{}{
		"tenantId": tenantID,
		"period":   req.Period,
		"metric":   req.Metric,
		"start":    startKey,
		"end":      endKey,
	}
	jsonData, _ := json.Marshal(cacheData)
	hash := md5.Sum(jsonData)
	return fmt.Sprintf("alunos_ranking:%s", hex.EncodeToString(hash[:]))
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
