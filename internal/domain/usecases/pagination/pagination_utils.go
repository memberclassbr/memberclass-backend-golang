package pagination

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type PaginationUtils struct{}

func NewPaginationUtils() *PaginationUtils {
	return &PaginationUtils{}
}

func (p *PaginationUtils) ParsePaginationFromQuery(queryParams map[string]string) *dto.PaginationRequest {
	req := &dto.PaginationRequest{
		Page:     1,
		PageSize: 10,
		SortBy:   "createdAt",
		SortDir:  "desc",
	}

	if pageStr, exists := queryParams["page"]; exists {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			req.Page = page
		}
	}

	if pageSizeStr, exists := queryParams["pageSize"]; exists {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 && pageSize <= 100 {
			req.PageSize = pageSize
		}
	}

	if sortBy, exists := queryParams["sortBy"]; exists && sortBy != "" {
		req.SortBy = sortBy
	}

	if sortDir, exists := queryParams["sortDir"]; exists {
		sortDir = strings.ToLower(sortDir)
		if sortDir == "asc" || sortDir == "desc" {
			req.SortDir = sortDir
		}
	}

	return req
}

func (p *PaginationUtils) BuildSQLPagination(baseQuery string, req *dto.PaginationRequest) string {

	orderBy := fmt.Sprintf("ORDER BY %s %s", req.GetSortBy(), strings.ToUpper(req.GetSortDir()))

	limit := req.GetLimit()
	offset := req.GetOffset()

	return fmt.Sprintf("%s %s LIMIT %d OFFSET %d", baseQuery, orderBy, limit, offset)
}

func (p *PaginationUtils) BuildCountQuery(baseQuery string) string {

	query := strings.ToLower(baseQuery)

	if orderByIndex := strings.Index(query, " order by "); orderByIndex != -1 {
		baseQuery = baseQuery[:orderByIndex]
	}

	if limitIndex := strings.Index(query, " limit "); limitIndex != -1 {
		baseQuery = baseQuery[:limitIndex]
	}

	if offsetIndex := strings.Index(query, " offset "); offsetIndex != -1 {
		baseQuery = baseQuery[:offsetIndex]
	}

	return fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS count_query", baseQuery)
}

func (p *PaginationUtils) ValidatePaginationRequest(req *dto.PaginationRequest) {
	if req.Page <= 0 {
		req.Page = 1
	}

	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	if req.PageSize > 100 {
		req.PageSize = 100
	}

	if req.SortBy == "" {
		req.SortBy = "createdAt"
	}

	if req.SortDir == "" {
		req.SortDir = "desc"
	}

	req.SortDir = strings.ToLower(req.SortDir)
	if req.SortDir != "asc" && req.SortDir != "desc" {
		req.SortDir = "desc"
	}
}
