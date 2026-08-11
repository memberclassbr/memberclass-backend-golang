// Package session carries the NextAuth session of a frontend-originated
// request. The session middleware decrypts the cookie into a Payload and puts
// it in the context; handlers read it back.
package session

import "context"

// UserTenant is one tenant membership carried on the session.
type UserTenant struct {
	Role    string `json:"role"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// UserInfo is the account the session belongs to.
type UserInfo struct {
	ID       string       `json:"id"`
	Email    string       `json:"email"`
	Name     string       `json:"name"`
	Username string       `json:"username"`
	Image    string       `json:"image"`
	Tenants  []UserTenant `json:"tenants"`
}

// Payload is the decrypted NextAuth session token. The field names match
// NextAuth's JWT claims and must not be renamed.
type Payload struct {
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

type contextKey string

// ContextKey is where the decrypted session is stored.
const ContextKey contextKey = "user"

// FromContext returns the session, or nil when the request did not pass
// through the session middleware.
func FromContext(ctx context.Context) *Payload {
	payload, ok := ctx.Value(ContextKey).(*Payload)
	if !ok {
		return nil
	}
	return payload
}
