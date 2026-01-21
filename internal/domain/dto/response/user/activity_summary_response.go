package user

type UserActivitySummary struct {
	Email        string  `json:"email"`
	UltimoAcesso *string `json:"ultimoAcesso"`
}

type ActivitySummaryPagination struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	TotalCount  int  `json:"totalCount"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
	HasPrevPage bool `json:"hasPrevPage"`
}

type ActivitySummaryResponse struct {
	Users      []UserActivitySummary     `json:"users"`
	Pagination ActivitySummaryPagination `json:"pagination"`
}
