package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/middleware"
	"github.com/memberclass-backend-golang/internal/shared/tenantrole"
)

// ssoTokenTTL is how long a minted SSO token stays redeemable. It is short
// because the token travels in a URL.
const ssoTokenTTL = 5 * time.Minute

// tokenLength is the number of characters in the plaintext token.
const tokenLength = 32

// ---------- DTOs ----------

type generateTokenRequest struct {
	// UserID is the account the hand-off is for. Optional: omitted means the
	// caller themselves, which is the ordinary case. Naming somebody else is
	// an administrative act — see authorizeMint.
	UserID string `json:"userId"`
	// TenantID is the tenant the hand-off happens inside. Optional when the
	// caller belongs to exactly one — see resolveTenant for why it cannot
	// simply be dropped.
	TenantID string `json:"tenantId"`
}

type generateTokenResponse struct {
	Token         string `json:"token"`
	RedirectURL   string `json:"redirectUrl"`
	ExpiresInSecs int    `json:"expiresInSecs"`
}

type validateTokenRequest struct {
	Token string `json:"token"`
}

type ssoUserData struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Name     *string `json:"name"`
	Phone    *string `json:"phone"`
	Document *string `json:"document"`
}

type ssoTenantData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type validateTokenResponse struct {
	User   ssoUserData   `json:"user"`
	Tenant ssoTenantData `json:"tenant"`
}

// ---------- 1. HTTP handlers ----------

// GenerateSSOToken handles `POST /sso/generate-token?externalUrl=`.
//
// Authentication is the go-token Bearer JWT, applied by the router. Both body
// fields are optional and default to the caller: what this handler decides is
// who the caller is allowed to mint a hand-off *for*.
func (f *Feature) GenerateSSOToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// An absent body is a valid request — it means "a hand-off for me, in my
	// tenant" — so io.EOF is not a decoding failure here.
	var req generateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeCustomError(w, http.StatusBadRequest, "Requisição inválida", "INVALID_REQUEST")
		return
	}

	externalURL := r.URL.Query().Get("externalUrl")
	if externalURL == "" {
		writeCustomError(w, http.StatusBadRequest, "externalUrl é obrigatório", "INVALID_REQUEST")
		return
	}

	caller := middleware.GetAuthUser(r.Context())
	if caller == nil || caller.UserID == "" {
		f.writeAuthError(w, tenantrole.ErrNoIdentity)
		return
	}

	// Fill in what the caller left out before anything is judged, so the
	// permission check and the mint both work on the same resolved pair.
	if req.UserID == "" {
		req.UserID = caller.UserID
	}
	tenantID, err := f.resolveTenant(r.Context(), caller.UserID, req.TenantID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}
	req.TenantID = tenantID

	if err := f.authorizeMint(r.Context(), caller, req); err != nil {
		f.writeAuthError(w, err)
		return
	}

	resp, err := f.generateToken(r.Context(), req, externalURL)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ValidateSSOToken handles `POST /api/v1/sso/validate-token`.
func (f *Feature) ValidateSSOToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req validateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCustomError(w, http.StatusBadRequest, "Requisição inválida", "INVALID_REQUEST")
		return
	}
	if req.Token == "" {
		writeCustomError(w, http.StatusBadRequest, "token é obrigatório", "INVALID_REQUEST")
		return
	}

	resp, err := f.consumeToken(r.Context(), req.Token, clientIP(r))
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// clientIP prefers the proxy headers, since the service runs behind one, and
// falls back to the socket address. The IP is recorded against the redemption
// for audit.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// ---------- Auth: which tenant, and who may mint for whom ----------

// resolveTenant fills in the tenant when the caller did not name one.
//
// It cannot come off the token: the go-token JWT carries sub, email and role,
// and nothing that names a tenant — it identifies an account, not a
// membership. Nor is there always one answer to read, since "UsersOnTenants"
// is many-to-many and the same account can hold a different role in each.
//
// So: one membership means there is nothing to choose and the field is
// redundant; several means guessing would mint a hand-off into the wrong
// tenant, and the request is refused until it says which.
func (f *Feature) resolveTenant(ctx context.Context, callerID, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}

	ids, err := f.tenantsOf(ctx, callerID)
	if err != nil {
		f.log.Error("Error listing tenants for caller: " + err.Error())
		return "", &memberclasserrors.MemberClassError{Code: 500, Message: "erro ao resolver tenant"}
	}

	switch len(ids) {
	case 0:
		return "", &memberclasserrors.MemberClassError{Code: 403, Message: "usuário não pertence a nenhum tenant"}
	case 1:
		return ids[0], nil
	default:
		return "", &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "tenantId é obrigatório: o usuário pertence a mais de um tenant",
		}
	}
}

// authorizeMint decides whether the caller may mint an SSO hand-off for
// req.UserID inside req.TenantID.
//
// The endpoint is open to every role, but only for the caller's own account.
// The token this mints is redeemed at validate-token for the target user's
// identity — id, email, name, phone, document — on the tenant's external site,
// so minting one for somebody else is impersonation, not delegation. A member
// signing themselves out to the tenant's site is the actual use case and stays
// open to any role; acting on another account is an administrative act and
// takes owner or admin.
//
// Before this route moved to the Bearer, the whole endpoint sat behind
// x-internal-api-key and an arbitrary userId was safe because only the
// platform's own backend could reach it. That is no longer true.
func (f *Feature) authorizeMint(ctx context.Context, caller *middleware.AuthUser, req generateTokenRequest) error {
	allowed := tenantrole.AnyRole
	if caller.UserID != req.UserID {
		allowed = tenantrole.OwnerOrAdmin
	}
	_, err := f.roles.Authorize(ctx, req.TenantID, allowed...)
	return err
}

// writeAuthError maps a tenantrole failure onto this slice's error envelope,
// which is the `{ok, error, errorCode}` shape its other failures already use.
func (f *Feature) writeAuthError(w http.ResponseWriter, err error) {
	switch status := tenantrole.Status(err); status {
	case http.StatusUnauthorized:
		writeCustomError(w, status, "Não autorizado: token é obrigatório", "UNAUTHORIZED")
	case http.StatusForbidden:
		writeCustomError(w, status, "Sem permissão para gerar token para este usuário", "FORBIDDEN")
	default:
		f.log.Error("sso: role lookup failed: " + err.Error())
		writeCustomError(w, status, "Erro ao validar acesso ao tenant", "INTERNAL_ERROR")
	}
}

// ---------- 2. Business rules ----------

func (f *Feature) generateToken(ctx context.Context, req generateTokenRequest, externalURL string) (*generateTokenResponse, error) {
	// Existence and membership are reported differently — an unknown user is a
	// 404, a user outside the tenant a 403 — so they stay two checks.
	exists, err := f.userExists(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &memberclasserrors.MemberClassError{Code: 404, Message: "usuário não encontrado"}
	}

	belongs, err := f.userBelongsToTenant(ctx, req.UserID, req.TenantID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, &memberclasserrors.MemberClassError{Code: 403, Message: "usuário não pertence ao tenant"}
	}

	token, err := randomToken(tokenLength)
	if err != nil {
		f.log.Error("Error generating random token: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "erro ao gerar token"}
	}

	redirectURL, err := buildRedirectURL(externalURL, token)
	if err != nil {
		f.log.Error("Error building redirect URL: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{Code: 400, Message: "URL externa inválida"}
	}

	if err := f.storeToken(ctx, req.UserID, req.TenantID, hashToken(token), time.Now().UTC().Add(ssoTokenTTL)); err != nil {
		return nil, err
	}

	return &generateTokenResponse{
		Token:         token,
		RedirectURL:   redirectURL,
		ExpiresInSecs: int(ssoTokenTTL.Seconds()),
	}, nil
}

func randomToken(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func buildRedirectURL(externalURL, token string) (string, error) {
	parsed, err := url.Parse(externalURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("token-mc", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// ---------- 3. SQL ----------

const (
	sqlUserExists = `SELECT EXISTS(SELECT 1 FROM "User" WHERE id = $1)`

	sqlUserBelongsToTenant = `
		SELECT EXISTS(
			SELECT 1 FROM "UsersOnTenants"
			WHERE "userId" = $1 AND "tenantId" = $2
		)
	`

	// sqlTenantsOfUser backs the tenantId default. LIMIT 2 is the whole query:
	// the caller only needs to know "exactly one" from "more than one", and
	// listing every membership to then count them would be work nobody reads.
	sqlTenantsOfUser = `
		SELECT "tenantId"
		FROM "UsersOnTenants"
		WHERE "userId" = $1
		LIMIT 2
	`

	sqlStoreSSOToken = `
		UPDATE "UsersOnTenants"
		SET "ssoToken" = $1, "ssoTokenValidUntil" = $2, "ssoTokenUsedAt" = NULL, "ssoTokenIP" = NULL
		WHERE "userId" = $3 AND "tenantId" = $4
	`

	// sqlLockTokenRow takes a row lock so two concurrent redemptions of the
	// same token serialise; the second one then sees ssoTokenUsedAt set.
	sqlLockTokenRow = `
		SELECT
			uot."userId",
			uot."tenantId",
			uot."ssoTokenValidUntil"::timestamp,
			uot."ssoTokenUsedAt"::timestamp,
			u.email,
			uot.name,
			u.phone,
			t.name as tenant_name
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		JOIN "Tenant" t ON t.id = uot."tenantId"
		WHERE uot."ssoToken" = $1
		FOR UPDATE
	`

	sqlMarkTokenUsed = `
		UPDATE "UsersOnTenants"
		SET "ssoTokenUsedAt" = $1, "ssoTokenIP" = $2
		WHERE "userId" = $3 AND "tenantId" = $4
	`

	sqlUserDocument = `
		SELECT document
		FROM "UsersOnTenants"
		WHERE "userId" = $1 AND document IS NOT NULL
		LIMIT 1
	`
)

func (f *Feature) userExists(ctx context.Context, userID string) (bool, error) {
	var exists bool
	if err := f.db.QueryRowContext(ctx, sqlUserExists, userID).Scan(&exists); err != nil {
		return false, f.fail("Error checking user existence: ", err, "error checking user existence")
	}
	return exists, nil
}

func (f *Feature) userBelongsToTenant(ctx context.Context, userID, tenantID string) (bool, error) {
	var belongs bool
	if err := f.db.QueryRowContext(ctx, sqlUserBelongsToTenant, userID, tenantID).Scan(&belongs); err != nil {
		return false, f.fail("Error checking user tenant membership: ", err, "error checking user tenant membership")
	}
	return belongs, nil
}

// tenantsOf returns up to two of the caller's tenant memberships — enough to
// tell "exactly one" from "more than one", which is all resolveTenant asks.
func (f *Feature) tenantsOf(ctx context.Context, userID string) ([]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlTenantsOfUser, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (f *Feature) storeToken(ctx context.Context, userID, tenantID, tokenHash string, validUntil time.Time) error {
	result, err := f.db.ExecContext(ctx, sqlStoreSSOToken, tokenHash, validUntil, userID, tenantID)
	if err != nil {
		return f.fail("Error updating SSO token: ", err, "error updating SSO token")
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return f.fail("Error checking rows affected: ", err, "error checking rows affected")
	}
	if affected == 0 {
		return &memberclasserrors.MemberClassError{Code: 404, Message: "user not found in tenant"}
	}
	return nil
}

// consumeToken validates and burns the token in one transaction: the row is
// locked, checked for prior use and expiry, then marked used before commit.
func (f *Feature) consumeToken(ctx context.Context, token, ip string) (*validateTokenResponse, error) {
	tokenHash := hashToken(token)

	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, f.fail("Error starting transaction: ", err, "error starting transaction")
	}
	defer func() { _ = tx.Rollback() }()

	var userID, tenantID, email, tenantName string
	var name, phone *string
	var validUntil time.Time
	var usedAt sql.NullTime

	err = tx.QueryRowContext(ctx, sqlLockTokenRow, tokenHash).
		Scan(&userID, &tenantID, &validUntil, &usedAt, &email, &name, &phone, &tenantName)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, &memberclasserrors.MemberClassError{Code: 401, Message: "token inválido"}
	case err != nil:
		return nil, f.fail("Error validating SSO token: ", err, "error validating SSO token")
	}

	if usedAt.Valid {
		return nil, &memberclasserrors.MemberClassError{Code: 401, Message: "token já foi utilizado"}
	}
	if time.Now().UTC().After(validUntil.UTC()) {
		return nil, &memberclasserrors.MemberClassError{Code: 401, Message: "token expirado"}
	}

	if _, err := tx.ExecContext(ctx, sqlMarkTokenUsed, time.Now().UTC(), ip, userID, tenantID); err != nil {
		return nil, f.fail("Error marking token as used: ", err, "error marking token as used")
	}

	if err := tx.Commit(); err != nil {
		return nil, f.fail("Error committing transaction: ", err, "error committing transaction")
	}

	// The document lives on a different row and is optional; failing to read it
	// must not undo a redemption that already committed.
	document, err := f.userDocument(ctx, userID)
	if err != nil {
		f.log.Error("Error getting user document: " + err.Error())
	}

	return &validateTokenResponse{
		User:   ssoUserData{ID: userID, Email: email, Name: name, Phone: phone, Document: document},
		Tenant: ssoTenantData{ID: tenantID, Name: tenantName},
	}, nil
}

func (f *Feature) userDocument(ctx context.Context, userID string) (*string, error) {
	var document sql.NullString
	err := f.db.QueryRowContext(ctx, sqlUserDocument, userID).Scan(&document)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}
	if !document.Valid {
		return nil, nil
	}
	value := document.String
	return &value, nil
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
	case http.StatusUnauthorized:
		writeCustomError(w, http.StatusUnauthorized, mcErr.Message, "INVALID_TOKEN")
	case http.StatusForbidden:
		writeCustomError(w, http.StatusForbidden, mcErr.Message, "FORBIDDEN")
	case http.StatusNotFound:
		writeCustomError(w, http.StatusNotFound, mcErr.Message, "NOT_FOUND")
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
