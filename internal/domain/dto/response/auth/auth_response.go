package auth

type AuthResponse struct {
	OK   bool   `json:"ok"`
	Link string `json:"link"`
}
