package response

type PostSocialResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Platform  string `json:"platform"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

