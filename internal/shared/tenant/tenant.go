// Package tenant carries the authenticated tenant through a request.
//
// The auth middleware resolves the API key to a tenant and puts it in the
// context; every slice reads it back to scope its queries. The struct holds
// only what that path needs — a slice that wants more columns queries for them
// itself rather than widening this type.
package tenant

import "context"

// Tenant identifies the customer a request belongs to.
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// contextKey is unexported so nothing outside this package can write the value
// the middleware owns.
type contextKey string

// ContextKey is where the authenticated tenant is stored.
const ContextKey contextKey = "tenant"

// FromContext returns the authenticated tenant, or nil when the request did
// not pass through the auth middleware. Handlers must treat nil as
// unauthenticated rather than dereferencing it.
func FromContext(ctx context.Context) *Tenant {
	found, ok := ctx.Value(ContextKey).(*Tenant)
	if !ok {
		return nil
	}
	return found
}

// NewContext attaches t to ctx. Intended for the auth middleware and for
// tests; a handler must never call it to fabricate a tenant.
func NewContext(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, ContextKey, t)
}
