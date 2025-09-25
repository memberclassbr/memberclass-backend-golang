package entities

import "time"

type Tenant struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	CreatedAt               time.Time `json:"createdAt"`
	Description             string    `json:"description"`
	Plan                    string    `json:"plan"`
	EmailContact            string    `json:"emailContact"`
	Logo                    string    `json:"logo"`
	Image                   string    `json:"image"`
	Favicon                 string    `json:"favicon"`
	BgLogin                 string    `json:"bgLogin"`
	CustomMenu              string    `json:"customMenu"`
	ExternalCodes           string    `json:"externalCodes"`
	SubDomain               string    `json:"subdomain"`
	CustomDomain            string    `json:"customDomain"`
	MainColor               string    `json:"mainColor"`
	DropboxAppID            string    `json:"dropboxAppId"`
	DropboxMemberID         string    `json:"dropboxMemberId"`
	DropboxRefreshToken     string    `json:"dropboxRefreshToken"`
	DropboxAccessToken      string    `json:"dropboxAccessToken"`
	DropboxAccessTokenValid time.Time `json:"dropboxAccessTokenValid"`
	Import                  bool      `json:"import"`
	IsOpenArea              bool      `json:"isOpenArea"`
	ListFiles               bool      `json:"listFiles"`
	Comments                string    `json:"comments"`
	HideCards               bool      `json:"hideCards"`
	HideYoutube             bool      `json:"hideYoutube"`
	BunnyLibraryApiKey      string    `json:"bunnyLibraryApiKey"`
	BunnyLibraryID          string    `json:"bunnyLibraryId"`
	TokenApiAuth            string    `json:"token_api_auth"`
	Language                string    `json:"language"`
	WebhookAPI              string    `json:"webhook_api"`
	RegisterNewUser         bool      `json:"registerNewUser"`
	AIEnabled               bool      `json:"aiEnabled"`
}
