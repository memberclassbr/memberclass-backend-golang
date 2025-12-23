package response

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
	LastAccess *string         `json:"lastAccess"`
}

type UserInformationsPagination struct {
	Page            int  `json:"page"`
	TotalPages      int  `json:"totalPages"`
	TotalItems      int  `json:"totalItems"`
	ItemsPerPage    int  `json:"itemsPerPage"`
	HasNextPage     bool `json:"hasNextPage"`
	HasPreviousPage bool `json:"hasPreviousPage"`
}

type UserInformationsResponse struct {
	Users      []UserInformation          `json:"users"`
	Pagination UserInformationsPagination `json:"pagination"`
}
