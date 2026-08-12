// Package tenantrole answers one question for the routes the admin frontend
// calls with a go-token Bearer JWT: does the caller hold a role in the tenant
// they are acting on, and is that role enough for what they are asking to do?
//
// The JWT carries a `role` claim and this package deliberately ignores it.
// That claim is copied out of a NextAuth session that may predate a demotion,
// and it names no tenant at all — trusting it would let one token pass as the
// same role in every tenant the holder can name in a request body. The role is
// read from "UsersOnTenants" on every request instead.
//
// Authentication stays in middleware.BearerMiddleware; this package is only
// authorization. It is a helper rather than a middleware because the tenant
// being acted on arrives inside the request — JSON for the imports and the SSO
// hand-off, a multipart field for the video upload — and a middleware that
// read the body would have to hand it back unconsumed to the handler.
package tenantrole

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"

	"github.com/memberclass-backend-golang/internal/shared/middleware"
)

// The role names stored in "UsersOnTenants".role. Prisma declares the column as
// a free String rather than an enum, so this is the set the frontend writes
// today, not one the database enforces: compare against these names, never
// assume nothing else can appear.
const (
	Owner   = "owner"
	Admin   = "admin"
	Manager = "manager"
	Tutor   = "tutor"
	Member  = "member"
)

// OwnerOrAdmin is the set allowed to act on the tenant as a whole rather than
// on their own account.
var OwnerOrAdmin = []string{Owner, Admin}

// AnyRole is the empty whitelist: belonging to the tenant is the whole
// requirement. Spelled out so a call site reads as a decision rather than as a
// forgotten argument.
var AnyRole []string

var (
	// ErrNoIdentity means the request never carried a verified Bearer token.
	ErrNoIdentity = errors.New("tenantrole: no authenticated identity on the request")

	// ErrNotMember means the caller holds no row in the target tenant.
	ErrNotMember = errors.New("tenantrole: caller does not belong to the tenant")

	// ErrForbiddenRole means the caller belongs to the tenant, but their role
	// is not in the whitelist the endpoint asked for.
	ErrForbiddenRole = errors.New("tenantrole: caller's role is not allowed here")
)

// Grant is the caller's verified standing inside one tenant.
type Grant struct {
	UserID   string
	Email    string
	TenantID string
	// Role as the database holds it now, not as the token claimed it.
	Role string
}

// Checker reads roles out of the tenant database.
type Checker struct{ db *sql.DB }

// New builds a Checker over the tenant database.
func New(db *sql.DB) *Checker { return &Checker{db: db} }

// sqlRoleInTenant is scoped by both ids on purpose. The same account holds a
// different role in every tenant it belongs to, so a lookup by "userId" alone
// would answer a question nobody asked.
const sqlRoleInTenant = `
	SELECT role
	FROM "UsersOnTenants"
	WHERE "userId" = $1 AND "tenantId" = $2
	LIMIT 1
`

// Authorize checks the Bearer identity on ctx against tenantID.
//
// `allowed` is a whitelist of role names; pass AnyRole (no arguments) when
// membership alone is the requirement. A row whose role is empty counts as no
// role rather than as a wildcard.
//
// The returned error is one of this package's sentinels, or the driver's error
// when the lookup itself failed — map it with Status.
func (c *Checker) Authorize(ctx context.Context, tenantID string, allowed ...string) (*Grant, error) {
	user := middleware.GetAuthUser(ctx)
	if user == nil || user.UserID == "" {
		return nil, ErrNoIdentity
	}
	// An absent tenant cannot be checked, and answering "not a member" is the
	// truthful reading: there is no tenant the caller belongs to here.
	if tenantID == "" {
		return nil, ErrNotMember
	}

	var role string
	err := c.db.QueryRowContext(ctx, sqlRoleInTenant, user.UserID, tenantID).Scan(&role)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotMember
	case err != nil:
		return nil, err
	}

	if role == "" {
		return nil, ErrForbiddenRole
	}
	if len(allowed) > 0 && !slices.Contains(allowed, role) {
		return nil, ErrForbiddenRole
	}

	return &Grant{UserID: user.UserID, Email: user.Email, TenantID: tenantID, Role: role}, nil
}

// Status maps this package's sentinels to the status code a handler should
// answer with. The body is left to the slice: this service has two error
// envelopes and both are contract, so which one an endpoint writes is settled
// by what that endpoint already returns.
func Status(err error) int {
	switch {
	case errors.Is(err, ErrNoIdentity):
		return http.StatusUnauthorized
	case errors.Is(err, ErrNotMember), errors.Is(err, ErrForbiddenRole):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
