package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

// listCacheTTL is how long a page of results is cached. Shared by all three
// actions, matching the previous use cases.
const listCacheTTL = 300 * time.Second

// timestampLayout is the format every timestamp in this slice's responses uses.
// It is not RFC3339: the fixed ".000Z" suffix is what clients already parse.
const timestampLayout = "2006-01-02T15:04:05.000Z"

// errUserNotInTenant is returned when the requested email does not resolve to a
// member of the authenticated tenant — whether the user does not exist at all
// or exists under a different tenant. The two cases are deliberately
// indistinguishable so the endpoint cannot be used to probe for accounts.
var errUserNotInTenant = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")

// sqlMemberByEmail resolves an email to a user id within one tenant.
const sqlMemberByEmail = `
	SELECT u.id
	FROM "User" u
	JOIN "UsersOnTenants" uot ON uot."userId" = u.id
	WHERE u.email = $1 AND uot."tenantId" = $2
`

// memberID looks up the tenant member behind an email. A missing row is
// errUserNotInTenant; a database failure is a 500, not a not-found.
func (f *Feature) memberID(ctx context.Context, email, tenantID string) (string, error) {
	var userID string
	err := f.db.QueryRowContext(ctx, sqlMemberByEmail, email, tenantID).Scan(&userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", errUserNotInTenant
	case err != nil:
		return "", f.fail("Error finding user by email: ", err, "error finding user by email")
	}
	return userID, nil
}

// ---------- shared parsing ----------

// parsePage reads ?page, defaulting to 1. The bad-input message is the one the
// endpoints have always returned.
func parsePage(query map[string][]string) (int, error) {
	raw := first(query, "page")
	if raw == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		return 0, errors.New("page must be a positive integer")
	}
	return page, nil
}

// parseLimit reads ?limit, defaulting to 10 and capped at 100.
func parseLimit(query map[string][]string) (int, error) {
	raw := first(query, "limit")
	if raw == "" {
		return 10, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func first(query map[string][]string, key string) string {
	values := query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func timePtr(v sql.NullTime) *string {
	if !v.Valid {
		return nil
	}
	formatted := v.Time.Format(timestampLayout)
	return &formatted
}

// stringPtr keeps a NULL column null in the response instead of flattening it
// to "". For the payment fields on a delivery the difference carries meaning: a
// grant created by hand has no platform, and reporting one as an empty string
// makes it indistinguishable from a gateway grant that lost its origin.
func stringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// ---------- errors ----------

// fail logs the driver error and returns the 500 the client sees.
func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

// writeUnexpected handles anything the action-specific mapping did not claim.
func (f *Feature) writeUnexpected(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if errors.As(err, &mcErr) {
		writeError(w, mcErr.Code, mcErr.Message)
		return
	}
	f.log.Error("Unexpected error: " + err.Error())
	writeError(w, http.StatusInternalServerError, "Internal server error")
}

// ---------- responses ----------

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError uses the {error, message} shape. `message` is omitted when empty,
// matching the DTO these endpoints used.
func writeError(w http.ResponseWriter, code int, message string) {
	body := struct {
		Error   string `json:"error"`
		Message string `json:"message,omitempty"`
	}{Error: http.StatusText(code), Message: message}
	writeJSON(w, code, body)
}

// writeCustomError uses the {ok, error, errorCode} shape, for the failures
// clients switch on by code.
func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
