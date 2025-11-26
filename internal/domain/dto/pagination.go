package dto


type PaginationRequest struct {
	Page     int    `json:"page" form:"page" validate:"min=1"`
	PageSize int    `json:"pageSize" form:"pageSize" validate:"min=1,max=100"`
	SortBy   string `json:"sortBy" form:"sortBy"`
	SortDir  string `json:"sortDir" form:"sortDir" validate:"oneof=asc desc"`
}


type PaginationResponse[T any] struct {
	Data       []T  `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}


type PaginationMeta struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"pageSize"`
	Total      int64 `json:"total"`
	TotalPages int  `json:"totalPages"`
	HasNext    bool `json:"hasNext"`
	HasPrev    bool `json:"hasPrev"`
}


func (p *PaginationRequest) GetOffset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize
}


func (p *PaginationRequest) GetLimit() int {
	if p.PageSize <= 0 {
		p.PageSize = 10
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p.PageSize
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
	pageSize := req.GetLimit()
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	
	return &PaginationResponse[T]{
		Data: data,
		Pagination: PaginationMeta{
			Page:       req.Page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
			HasNext:    req.Page < totalPages,
			HasPrev:    req.Page > 1,
		},
	}
}
