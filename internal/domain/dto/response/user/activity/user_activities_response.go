package activity

import "github.com/memberclass-backend-golang/internal/domain/dto"

type AccessData struct {
	Data string `json:"data"`
}

type ActivityResponse struct {
	Email      string             `json:"email"`
	Access     []AccessData       `json:"access"`
	Pagination dto.PaginationMeta `json:"pagination"`
}
