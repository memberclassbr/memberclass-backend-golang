package user

type DeliveryInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AccessDate string `json:"accessDate"`
}

type UserInformation struct {
	UserID     string         `json:"userId"`
	Email      string         `json:"email"`
	IsPaid     bool           `json:"isPaid"`
	Deliveries []DeliveryInfo `json:"deliveries"`
	LastAccess *string        `json:"lastAccess"`
}

type UserInformationsPagination struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	TotalCount  int  `json:"totalCount"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
	HasPrevPage bool `json:"hasPrevPage"`
}

type UserInformationsResponse struct {
	Users      []UserInformation          `json:"users"`
	Pagination UserInformationsPagination `json:"pagination"`
}
