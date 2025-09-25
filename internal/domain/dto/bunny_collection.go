package dto

type BunnyCollection struct {
	GUID       string `json:"guid"`
	Name       string `json:"name"`
	VideoCount int    `json:"videoCount"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}
