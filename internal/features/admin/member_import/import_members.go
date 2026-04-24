package member_import

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/application/middlewares/auth"
	"github.com/memberclass-backend-golang/internal/domain/utils"
)

// ---------- Request/response DTOs ----------

type deliveryRef struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type importUserInput struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone,omitempty"`
	Accession string `json:"accession,omitempty"`
	Document  string `json:"document,omitempty"`
}

type importRequest struct {
	TenantID    string            `json:"tenantId"`
	FileName    string            `json:"fileName"`
	Users       []importUserInput `json:"users"`
	Deliveries  []deliveryRef     `json:"deliveries"`
	PassDefault string            `json:"passDefault,omitempty"`
}

type importAcceptedResponse struct {
	ImportID string `json:"importId"`
	Status   string `json:"status"`
}

// tenantRow is the subset of "Tenant" columns the processor needs to render
// emails. Kept slice-local to avoid importing the full tenant entity.
type tenantRow struct {
	ID              string
	Name            string
	Subdomain       sql.NullString
	CustomDomain    sql.NullString
	Logo            sql.NullString
	MainColor       sql.NullString
	BackgroundColor sql.NullString
	TextColor       sql.NullString
	EmailContact    sql.NullString
	Language        sql.NullString
}

// ---------- HTTP handler ----------

// ImportMembers handles `POST /imports/members`.
//
// Flow:
//  1. Decode body, basic shape validation.
//  2. Resolve session from context (put there by the auth middleware).
//  3. Query UsersOnTenants to confirm the session user belongs to tenantId
//     with role != "member".
//  4. Load the tenant row (needed to build email links + batch emails).
//  5. INSERT a "UserImport" header with status="processing" and respond 202
//     immediately with { importId }.
//  6. Spawn a goroutine (panic-recovered) that processes the full job.
func (f *Feature) ImportMembers(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if err := validateRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	authUser := auth.GetAuthUser(r.Context())
	if authUser == nil || authUser.UserID == "" {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	role, err := f.loadRoleForTenant(r.Context(), authUser.UserID, req.TenantID)
	if err != nil {
		if errors.Is(err, errNotMember) {
			writeError(w, http.StatusForbidden, "user does not belong to tenant")
			return
		}
		f.log.Error("import: role lookup failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "failed to validate tenant access")
		return
	}
	if role == "" || role == "member" {
		writeError(w, http.StatusForbidden, "insufficient role")
		return
	}

	tenant, err := f.loadTenant(r.Context(), req.TenantID)
	if err != nil {
		f.log.Error("import: tenant load failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "failed to load tenant")
		return
	}

	importID, err := f.createImportHeader(r.Context(), authUser.UserID, &req)
	if err != nil {
		f.log.Error("import: create header failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "failed to start import")
		return
	}

	f.log.Info("import.started",
		"import_id", importID,
		"tenant_id", req.TenantID,
		"total_rows", len(req.Users),
	)

	// Fire-and-forget worker; the request's context gets cancelled as soon
	// as this handler returns, so spawn with a fresh background context.
	// `inflight` lets `Feature.Wait` drain this goroutine on shutdown
	// before the DB closes — Add MUST happen here (not inside the goroutine)
	// to avoid a race with a shutdown that begins between `go` and the first
	// line of the closure.
	f.inflight.Add(1)
	go func() {
		defer f.inflight.Done()
		f.runImport(importID, &req, tenant)
	}()

	writeJSON(w, http.StatusAccepted, importAcceptedResponse{
		ImportID: importID,
		Status:   "processing",
	})
}

// ---------- Validation ----------

// Hard limits to prevent a compromised admin token from tying up the
// import worker for hours or blowing past Postgres' 65k-parameter
// protocol limit in `insertMemberOnDeliveries` (100 users × D deliveries
// × 4 params per row).
const (
	maxImportUsers      = 10_000
	maxImportDeliveries = 50
)

func validateRequest(req *importRequest) error {
	if req.TenantID == "" {
		return errors.New("tenantId is required")
	}
	if len(req.Users) == 0 {
		return errors.New("users is empty")
	}
	if len(req.Users) > maxImportUsers {
		return fmt.Errorf("users exceeds max of %d", maxImportUsers)
	}
	if len(req.Deliveries) > maxImportDeliveries {
		return fmt.Errorf("deliveries exceeds max of %d", maxImportDeliveries)
	}
	for i, d := range req.Deliveries {
		if d.Value == "" {
			return fmt.Errorf("deliveries[%d].value is required", i)
		}
	}
	return nil
}

// ---------- Auth: role lookup ----------

var errNotMember = errors.New("user is not a member of tenant")

func (f *Feature) loadRoleForTenant(ctx context.Context, userID, tenantID string) (string, error) {
	const q = `
		SELECT role
		FROM "UsersOnTenants"
		WHERE "userId" = $1 AND "tenantId" = $2
		LIMIT 1
	`
	var role string
	err := f.db.QueryRowContext(ctx, q, userID, tenantID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errNotMember
		}
		return "", err
	}
	return role, nil
}

// ---------- Tenant load ----------

func (f *Feature) loadTenant(ctx context.Context, tenantID string) (*tenantRow, error) {
	const q = `
		SELECT id, name, subdomain, "customDomain", logo, "mainColor",
		       "backgroundColor", "textColor", "emailContact", language
		FROM "Tenant"
		WHERE id = $1
		LIMIT 1
	`
	t := &tenantRow{}
	err := f.db.QueryRowContext(ctx, q, tenantID).Scan(
		&t.ID, &t.Name, &t.Subdomain, &t.CustomDomain, &t.Logo,
		&t.MainColor, &t.BackgroundColor, &t.TextColor, &t.EmailContact, &t.Language,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ---------- UserImport header insert ----------

func (f *Feature) createImportHeader(ctx context.Context, userID string, req *importRequest) (string, error) {
	importID := utils.GenerateCUID()

	deliveriesJSON, err := json.Marshal(req.Deliveries)
	if err != nil {
		return "", fmt.Errorf("marshal deliveries: %w", err)
	}

	const q = `
		INSERT INTO "UserImport"
			(id, "tenantId", "importedByUserId", "fileName", status,
			 deliveries, "passDefault", "totalRows", "startedAt")
		VALUES ($1, $2, $3, $4, 'processing', $5::jsonb, $6, $7, NOW())
	`
	_, err = f.db.ExecContext(ctx, q,
		importID,
		req.TenantID,
		userID,
		req.FileName,
		deliveriesJSON,
		req.PassDefault != "",
		len(req.Users),
	)
	if err != nil {
		return "", err
	}
	return importID, nil
}

// ---------- HTTP helpers ----------

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

// tenantDomain picks the public domain for this tenant, matching the Next.js
// logic: customDomain wins outright; otherwise build `<subdomain>.<rootDomain>`
// where rootDomain is PUBLIC_DOMAIN_URL (the customer-facing frontend host,
// NOT the backend's own PUBLIC_ROOT_DOMAIN).
//
// Defensive: normalizeEmailDomain strips any scheme/port/path from
// `t.CustomDomain` so if a tenant admin saved it as "https://app.acme.com/"
// we still end up with "app.acme.com".
func tenantDomain(t *tenantRow, rootDomain string) string {
	if t.CustomDomain.Valid && t.CustomDomain.String != "" {
		if d := normalizeEmailDomain(t.CustomDomain.String); d != "" {
			return d
		}
	}
	sub := ""
	if t.Subdomain.Valid {
		sub = t.Subdomain.String
	}
	if sub == "" {
		return rootDomain
	}
	return sub + "." + rootDomain
}

// pickProtocol decides http vs https for the magic-link base URL based on
// the domain shape. Only bare localhost (with or without port) maps to http
// — we anchor on the full token to avoid `localhostfoo.com` sliding in.
func pickProtocol(domain string) string {
	if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
		return "http"
	}
	return "https"
}

// parseAccession parses the Next.js `dd/MM/yyyy HH:mm:ss` format; falls back
// to now on any parse error (keeps the row going — the timestamp is not
// business-critical enough to reject the import). Logs at debug so malformed
// client input is noticed without spamming the error channel.
func (f *Feature) parseAccession(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse("02/01/2006 15:04:05", s)
	if err != nil {
		f.log.Debug("import.parse_accession_fallback",
			"raw", s,
			"error", err.Error(),
		)
		return time.Now().UTC()
	}
	return t
}
