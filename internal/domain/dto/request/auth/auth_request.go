package auth

type AuthRequest struct {
	Email string `json:"email"`
}

func (r *AuthRequest) Validate() error {
	if r.Email == "" {
		return &ValidationError{Field: "email", Message: "Email é obrigatório e deve ser uma string"}
	}
	return nil
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
