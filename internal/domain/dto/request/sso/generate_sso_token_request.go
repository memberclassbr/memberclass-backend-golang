package sso

type GenerateSSOTokenRequest struct {
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId"`
}
