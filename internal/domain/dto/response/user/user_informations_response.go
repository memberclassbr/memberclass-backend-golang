package user

import "github.com/memberclass-backend-golang/internal/domain/dto"

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

type UserInformationsResponse struct {
	Users      []UserInformation `json:"users"`
	Pagination dto.PaginationMeta `json:"pagination"`
}
