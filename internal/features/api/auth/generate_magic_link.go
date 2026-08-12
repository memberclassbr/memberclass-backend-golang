package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// magicTokenTTL is how long a minted link stays usable.
const magicTokenTTL = 7 * 24 * time.Hour

// linkCacheTTL deduplicates repeated requests for the same member: within this
// window the same link is returned rather than invalidating the previous one.
const linkCacheTTL = 300 * time.Second

// bcryptCost is the work factor for hashing the magic token before storage.
const bcryptCost = 10

// linkFormat is the login URL the frontend serves.
const linkFormat = "%s://%s/login?token=%s&email=%s&isReset=false"

// ---------- DTOs ----------

type authRequest struct {
	Email string `json:"email"`
	// RedirectPath is where the frontend should land the member after it
	// exchanges the token for a session. Optional; a same-site path only.
	RedirectPath string `json:"redirectPath"`
}

type authResponse struct {
	OK   bool   `json:"ok"`
	Link string `json:"link"`
}

// ---------- 1. HTTP handler ----------

// GenerateMagicLink handles `POST /api/v1/auth`.
func (f *Feature) GenerateMagicLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCustomError(w, http.StatusBadRequest, msgEmailRequired, "INVALID_REQUEST")
		return
	}

	resp, err := f.generateMagicLink(r.Context(), req.Email, req.RedirectPath, tenant.ID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

const msgEmailRequired = "Email é obrigatório e deve ser uma string"

// ---------- 2. Business rule ----------

func (f *Feature) generateMagicLink(ctx context.Context, email, redirectPath, tenantID string) (*authResponse, error) {
	if email == "" {
		return nil, &memberclasserrors.MemberClassError{Code: 400, Message: msgEmailRequired}
	}

	redirect, err := sanitizeRedirectPath(redirectPath)
	if err != nil {
		return nil, err
	}

	// Repeated requests inside the cache window return the link already minted,
	// so a member who clicks "send me a link" twice does not invalidate the
	// first one before it arrives.
	cacheKey := fmt.Sprintf("auth_cache:%s:%s", tenantID, email)
	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		return &authResponse{OK: true, Link: withRedirect(cached, redirect)}, nil
	}

	userID, err := f.memberID(ctx, email, tenantID)
	if err != nil {
		return nil, err
	}

	token, err := generateToken()
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error generating magic token"}
	}

	// Only the hash is stored: a database dump must not yield usable links.
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "error hashing magic token"}
	}

	if err := f.storeMagicToken(ctx, userID, string(tokenHash), time.Now().Add(magicTokenTTL)); err != nil {
		return nil, err
	}

	link, err := f.buildLoginLink(ctx, tenantID, token, email)
	if err != nil {
		return nil, err
	}

	// The link is cached without the redirect, so two requests that differ only
	// in destination share one token — see withRedirect.
	if err := f.cache.Set(ctx, cacheKey, link, linkCacheTTL); err != nil {
		f.log.Error("Error caching auth response: " + err.Error())
	}

	return &authResponse{OK: true, Link: withRedirect(link, redirect)}, nil
}

// maxRedirectPathLen bounds the destination so a caller cannot push an
// arbitrarily large value into the link.
const maxRedirectPathLen = 512

const msgInvalidRedirect = `redirectPath deve ser um caminho relativo começando com "/"`

// sanitizeRedirectPath accepts only a same-site relative path, and returns the
// empty string when none was asked for.
//
// Everything a browser could resolve to another origin is rejected. This value
// ends up in a link the frontend navigates to right after it establishes a
// session, so an unchecked value turns the magic link into an open redirect —
// a phishing page on the attacker's host, reached from the tenant's own login.
func sanitizeRedirectPath(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	invalid := &memberclasserrors.MemberClassError{Code: 400, Message: msgInvalidRedirect}

	if len(raw) > maxRedirectPathLen {
		return "", invalid
	}
	// A backslash is not a path separator here, but browsers normalise `/\host`
	// into `//host`, so it has to go before the `//` test below means anything.
	if strings.Contains(raw, `\`) {
		return "", invalid
	}
	// A leading `//` is protocol-relative: `//evil.com` is another origin.
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", invalid
	}
	// Catches a scheme, an authority, and the ASCII control characters that
	// could smuggle a header break into the link.
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return "", invalid
	}

	return raw, nil
}

// withRedirect appends the post-login destination to a login link.
//
// It is applied after the cache rather than before it because "User"."magicToken"
// holds a single value per member: minting a second token for the same member
// invalidates the first. Caching the link without the destination lets two
// requests that differ only in where they land share one still-valid token.
func withRedirect(link, redirectPath string) string {
	if redirectPath == "" {
		return link
	}
	return link + "&redirect=" + url.QueryEscape(redirectPath)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// buildLoginLink resolves the tenant's own domain. A tenant with a custom
// domain uses it directly; otherwise the link is built from its subdomain
// under this deployment's root domain, defaulting to "acessos".
func (f *Feature) buildLoginLink(ctx context.Context, tenantID, token, email string) (string, error) {
	customDomain, subDomain, err := f.tenantDomains(ctx, tenantID)
	if err != nil {
		return "", err
	}

	domain := customDomain
	if domain == "" {
		subdomain := subDomain
		if subdomain == "" {
			subdomain = "acessos"
		}
		domain = fmt.Sprintf("%s.%s", subdomain, f.rootDomain)
	}

	protocol := "https"
	if strings.Contains(f.rootDomain, "localhost") {
		protocol = "http"
	}

	return fmt.Sprintf(linkFormat, protocol, domain, token, email), nil
}

// ---------- 3. SQL ----------

const (
	// sqlMemberByEmail resolves an email to a member of this tenant. Scoping
	// the lookup by tenant means one tenant cannot mint a link for another
	// tenant's user.
	sqlMemberByEmail = `
		SELECT u.id
		FROM "User" u
		JOIN "UsersOnTenants" uot ON uot."userId" = u.id
		WHERE u.email = $1 AND uot."tenantId" = $2
	`

	sqlStoreMagicToken = `
		UPDATE "User"
		SET "magicToken" = $1, "magicTokenValidUntil" = $2, "updatedAt" = NOW()
		WHERE id = $3
	`

	// Mind the casing: "customDomain" is camelCase but subdomain is not. Prisma
	// creates the column with the exact case of the field name, and the Tenant
	// model declares `subdomain`, so a quoted "subDomain" fails with
	// `column "subDomain" does not exist` at runtime — quoted identifiers are
	// matched exactly. member_import's tenant query has always had it right.
	sqlTenantDomains = `SELECT "customDomain", subdomain FROM "Tenant" WHERE id = $1`
)

func (f *Feature) memberID(ctx context.Context, email, tenantID string) (string, error) {
	var userID string
	err := f.db.QueryRowContext(ctx, sqlMemberByEmail, email, tenantID).Scan(&userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", &memberclasserrors.MemberClassError{Code: 404, Message: "Usuário não encontrado"}
	case err != nil:
		return "", f.fail("Error finding user by email: ", err, "error finding user by email")
	}
	return userID, nil
}

func (f *Feature) storeMagicToken(ctx context.Context, userID, tokenHash string, validUntil time.Time) error {
	_, err := f.db.ExecContext(ctx, sqlStoreMagicToken, tokenHash, validUntil, userID)
	if err != nil {
		return f.fail("Error updating magic token: ", err, "error updating magic token")
	}
	return nil
}

func (f *Feature) tenantDomains(ctx context.Context, tenantID string) (customDomain, subDomain string, err error) {
	var custom, sub sql.NullString
	err = f.db.QueryRowContext(ctx, sqlTenantDomains, tenantID).Scan(&custom, &sub)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", &memberclasserrors.MemberClassError{Code: 404, Message: "Tenant não encontrado"}
	case err != nil:
		return "", "", f.fail("Error finding tenant: ", err, "error building login link")
	}
	return custom.String, sub.String, nil
}

// ---------- errors and responses ----------

func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if !errors.As(err, &mcErr) || mcErr == nil {
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	switch mcErr.Code {
	case http.StatusBadRequest:
		writeCustomError(w, http.StatusBadRequest, mcErr.Message, "INVALID_REQUEST")
	case http.StatusNotFound:
		// The two 404s carry different codes: one means the address is unknown
		// to the platform, the other that it belongs to a different tenant.
		if mcErr.Message == "Usuário não encontrado" {
			writeCustomError(w, http.StatusNotFound, mcErr.Message, "USER_NOT_FOUND")
		} else {
			writeCustomError(w, http.StatusNotFound, mcErr.Message, "USER_NOT_IN_TENANT")
		}
	case http.StatusTooManyRequests:
		writeCustomError(w, http.StatusTooManyRequests, mcErr.Message, "RATE_LIMIT_EXCEEDED")
	default:
		writeError(w, mcErr.Code, mcErr.Message)
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	})
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
