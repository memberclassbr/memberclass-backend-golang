package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

// ---------- DTOs ----------

type aiTenantData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AIEnabled bool   `json:"aiEnabled"`
	// The Bunny library credentials travel in this response because the AI
	// dashboard needs them to build video links. The endpoint is internal-only
	// for that reason.
	BunnyLibraryID     *string `json:"bunnyLibraryId"`
	BunnyLibraryApiKey *string `json:"bunnyLibraryApiKey"`
}

type aiTenantsResponse struct {
	Tenants []aiTenantData `json:"tenants"`
	Total   int            `json:"total"`
}

// ---------- 1. HTTP handler ----------

// GetTenantsWithAIEnabled handles `GET /api/v1/ai/tenants`.
func (f *Feature) GetTenantsWithAIEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !f.authorized(w, r) {
		return
	}

	tenants, err := f.queryTenantsWithAI(r.Context())
	if err != nil {
		f.writeTenantsError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, &aiTenantsResponse{Tenants: tenants, Total: len(tenants)})
}

// ---------- 2. SQL ----------

const sqlTenantsWithAI = `
	SELECT id, name, "aiEnabled", "bunnyLibraryId", "bunnyLibraryApiKey"
	FROM "Tenant"
	WHERE "aiEnabled" = true
`

func (f *Feature) queryTenantsWithAI(ctx context.Context) ([]aiTenantData, error) {
	rows, err := f.db.QueryContext(ctx, sqlTenantsWithAI)
	if err != nil {
		return nil, f.fail("Error finding tenants with AI enabled: ", err, "error finding tenants with AI enabled")
	}
	defer rows.Close()

	tenants := make([]aiTenantData, 0)
	for rows.Next() {
		var t aiTenantData
		var bunnyLibraryID, bunnyLibraryApiKey sql.NullString

		if err := rows.Scan(&t.ID, &t.Name, &t.AIEnabled, &bunnyLibraryID, &bunnyLibraryApiKey); err != nil {
			return nil, f.fail("Error scanning tenant: ", err, "error scanning tenant")
		}

		t.BunnyLibraryID = strPtr(bunnyLibraryID)
		t.BunnyLibraryApiKey = strPtr(bunnyLibraryApiKey)
		tenants = append(tenants, t)
	}

	if err := rows.Err(); err != nil {
		return nil, f.fail("Error iterating tenants: ", err, "error iterating tenants")
	}

	return tenants, nil
}

// ---------- errors ----------

// writeTenantsError maps failures for this endpoint. It differs from the
// lessons mapping: only the rate-limit code is special-cased here.
func (f *Feature) writeTenantsError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if !errors.As(err, &mcErr) || mcErr == nil {
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	if mcErr.Code == http.StatusTooManyRequests {
		writeCustomError(w, http.StatusTooManyRequests, mcErr.Message, "RATE_LIMIT_EXCEEDED")
		return
	}
	writeError(w, mcErr.Code, mcErr.Message)
}
