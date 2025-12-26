package response

type AITenantsResponse struct {
	Tenants []AITenantData `json:"tenants"`
	Total   int            `json:"total"`
}

type AITenantData struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	AIEnabled          bool    `json:"aiEnabled"`
	BunnyLibraryID     *string `json:"bunnyLibraryId"`
	BunnyLibraryApiKey *string `json:"bunnyLibraryApiKey"`
}

