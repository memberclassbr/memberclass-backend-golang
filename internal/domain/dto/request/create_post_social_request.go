package request

import "errors"

type CreatePostSocialRequest struct {
	Email    string `json:"email"`
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

func (r *CreatePostSocialRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email é obrigatório")
	}
	if r.Platform == "" {
		return errors.New("platform é obrigatório")
	}
	if r.URL == "" {
		return errors.New("url é obrigatório")
	}
	return nil
}

