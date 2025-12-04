package dto

import "context"

type contextKey string

const UserContextKey contextKey = "user"

type UserTenant struct {
	Role    string `json:"role"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

type UserInfo struct {
	ID       string       `json:"id"`
	Email    string       `json:"email"`
	Name     string       `json:"name"`
	Username string       `json:"username"`
	Image    string       `json:"image"`
	Tenants  []UserTenant `json:"tenants"`
}

type SessionPayload struct {
	Sub   string   `json:"sub"`
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Image string   `json:"image"`
	Iat   int64    `json:"iat"`
	Exp   int64    `json:"exp"`
	Jti   string   `json:"jti"`
	Role  string   `json:"role"`
	User  UserInfo `json:"user"`
}

func GetUserFromContext(ctx context.Context) *SessionPayload {
	user, ok := ctx.Value(UserContextKey).(*SessionPayload)
	if !ok {
		return nil
	}
	return user
}

