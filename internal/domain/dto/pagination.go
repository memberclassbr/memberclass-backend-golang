package dto


type PaginationRequest struct {
	Page    int    `json:"page" form:"page" validate:"min=1"`
	Limit   int    `json:"limit" form:"limit" validate:"min=1,max=100"`
	SortBy  string `json:"sortBy" form:"sortBy"`
	SortDir string `json:"sortDir" form:"sortDir" validate:"oneof=asc desc"`
}


type PaginationResponse[T any] struct {
	Data       []T  `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}


type PaginationMeta struct {
	Page        int   `json:"page"`
	Limit       int   `json:"limit"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	HasNextPage bool  `json:"hasNextPage"`
	HasPrevPage bool  `json:"hasPrevPage"`
}


func (p *PaginationRequest) GetOffset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	return (p.Page - 1) * p.Limit
}


func (p *PaginationRequest) GetLimit() int {
	if p.Limit <= 0 {
		p.Limit = 10
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	return p.Limit
}


func (p *PaginationRequest) GetSortBy() string {
	if p.SortBy == "" {
		return "createdAt"
	}
	return p.SortBy
}


func (p *PaginationRequest) GetSortDir() string {
	if p.SortDir == "" {
		return "desc"
	}
	return p.SortDir
}


func NewPaginationResponse[T any](data []T, total int64, req *PaginationRequest) *PaginationResponse[T] {
	limit := req.GetLimit()
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	
	return &PaginationResponse[T]{
		Data: data,
		Pagination: PaginationMeta{
			Page:        req.Page,
			Limit:       limit,
			TotalCount:  total,
			TotalPages:  totalPages,
			HasNextPage: req.Page < totalPages,
			HasPrevPage: req.Page > 1,
		},
	}
}
