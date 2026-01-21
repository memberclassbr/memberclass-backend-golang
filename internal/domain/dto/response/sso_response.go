package response

type GenerateSSOTokenResponse struct {
	Token         string `json:"token"`
	RedirectURL   string `json:"redirectUrl"`
	ExpiresInSecs int    `json:"expiresInSecs"`
}

type ValidateSSOTokenResponse struct {
	User   SSOUserData   `json:"user"`
	Tenant SSOTenantData `json:"tenant"`
}

type SSOUserData struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Name     *string `json:"name"`
	Phone    *string `json:"phone"`
	Document *string `json:"document"`
}

type SSOTenantData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
