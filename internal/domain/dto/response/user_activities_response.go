package response

type AccessData struct {
	Data string `json:"data"`
}

type Pagination struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	TotalCount  int  `json:"totalCount"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
	HasPrevPage bool `json:"hasPrevPage"`
}

type ActivityResponse struct {
	Email      string       `json:"email"`
	Access     []AccessData `json:"access"`
	Pagination Pagination   `json:"pagination"`
}
