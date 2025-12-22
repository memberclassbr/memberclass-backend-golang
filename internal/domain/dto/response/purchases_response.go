package response

type UserPurchaseData struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type UserPurchasesResponse struct {
	Purchases  []UserPurchaseData `json:"purchases"`
	Pagination Pagination         `json:"pagination"`
}
