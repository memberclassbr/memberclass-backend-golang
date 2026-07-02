package user

import "github.com/memberclass-backend-golang/internal/domain/dto"

type DeliveryInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AccessDate string  `json:"accessDate"`
	Status     string  `json:"status"`    // active | expired | refunded | canceled
	ExpiresAt  *string `json:"expiresAt"` // null = vitalício (compra única)
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
