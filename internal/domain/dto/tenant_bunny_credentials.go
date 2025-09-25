package dto

type TenantBunnyCredentials struct {
	TenantID           string `json:"tenant_id"`
	BunnyLibraryApiKey string `json:"bunny_library_api_key"`
	BunnyLibraryID     string `json:"bunny_library_id"`
}
