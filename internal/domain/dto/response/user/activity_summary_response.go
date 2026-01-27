package user

import "github.com/memberclass-backend-golang/internal/domain/dto"

type UserActivitySummary struct {
	Email        string  `json:"email"`
	UltimoAcesso *string `json:"ultimoAcesso"`
}

type ActivitySummaryResponse struct {
	Users      []UserActivitySummary `json:"users"`
	Pagination dto.PaginationMeta    `json:"pagination"`
}
