package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/shared/pagination"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// ---------- DTOs ----------

type userPurchaseData struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type userPurchasesResponse struct {
	Purchases  []userPurchaseData `json:"purchases"`
	Pagination pagination.Meta    `json:"pagination"`
}

// ---------- 1. HTTP handler ----------

// successorPath is where callers of this endpoint should go instead.
const successorPath = "/api/v1/users/payment-events"

// GetUserPurchases handles `GET /api/v1/users/purchases`: the purchase and
// refund events recorded for one member of the tenant.
//
// Deprecated: use GetPaymentEvents. This reads the older UserEvent table, which
// records that something happened and when, and nothing else — its updatedAt is
// a copy of createdAt rather than a second timestamp. The replacement reads
// PaymentEvent and can say what was paid, on which platform and until when the
// access runs.
//
// It stays mounted and unchanged because the frontends calling it have not
// moved. The response is identical; only the advisory headers are new.
func (f *Feature) GetUserPurchases(w http.ResponseWriter, r *http.Request) {
	// RFC 8594 style: the header marks the endpoint deprecated and the link
	// names its replacement, so a client sees the notice without anyone having
	// to read the changelog.
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `<`+successorPath+`>; rel="successor-version"`)

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		writeCustomError(w, http.StatusBadRequest, "email é obrigatório", "MISSING_EMAIL")
		return
	}

	page, err := parsePage(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "Tenant not found in context")
		return
	}

	resp, err := f.getPurchases(r.Context(), tenant.ID, email, r.URL.Query().Get("type"), page, limit)
	if err != nil {
		if errors.Is(err, errUserNotInTenant) {
			writeCustomError(w, http.StatusNotFound, errUserNotInTenant.Error(), "USER_NOT_FOUND")
			return
		}
		f.writeUnexpected(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) getPurchases(ctx context.Context, tenantID, email, purchaseType string, page, limit int) (*userPurchasesResponse, error) {
	userID, err := f.memberID(ctx, email, tenantID)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("purchases:%s:%s:%d:%d:%s", tenantID, email, page, limit, purchaseType)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit userPurchasesResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	// No ?type means both kinds of money movement.
	types := []string{"purchase", "refund"}
	if purchaseType != "" {
		types = []string{purchaseType}
	}

	purchases, total, err := f.queryPurchases(ctx, userID, tenantID, types, page, limit)
	if err != nil {
		return nil, err
	}

	resp := &userPurchasesResponse{
		Purchases:  purchases,
		Pagination: pagination.NewMeta(page, limit, total),
	}

	if encoded, err := json.Marshal(resp); err == nil {
		if err := f.cache.Set(ctx, cacheKey, string(encoded), listCacheTTL); err != nil {
			f.log.Error(fmt.Sprintf("Error setting cache for key %s: %s", cacheKey, err.Error()))
		} else {
			f.log.Debug(fmt.Sprintf("Cache set for key: %s", cacheKey))
		}
	}

	return resp, nil
}

// ---------- 3. SQL ----------

// sqlPurchases pages the member's events and carries the unpaginated total on
// every row, so one round trip answers both questions. The timestamps are
// formatted in SQL to the exact layout the response uses.
const sqlPurchases = `
	WITH filtered AS (
		SELECT id, "createdAt", type
		FROM "UserEvent"
		WHERE "usersOnTenantsUserId" = $1
		  AND "usersOnTenantsTenantId" = $2
		  AND type = ANY($3)
	),
	paginated AS (
		SELECT * FROM filtered
		ORDER BY "createdAt" DESC
		LIMIT $4 OFFSET $5
	)
	SELECT
		p.id,
		p.type,
		TO_CHAR(p."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.000"Z"') as "createdAt",
		TO_CHAR(p."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.000"Z"') as "updatedAt",
		(SELECT COUNT(*) FROM filtered) as total_count
	FROM paginated p
`

func (f *Feature) queryPurchases(ctx context.Context, userID, tenantID string, types []string, page, limit int) ([]userPurchaseData, int64, error) {
	rows, err := f.db.QueryContext(ctx, sqlPurchases,
		userID, tenantID, pq.Array(types), limit, pagination.Offset(page, limit))
	if err != nil {
		return nil, 0, f.fail("Error finding purchases: ", err, "error finding purchases")
	}
	defer rows.Close()

	purchases := make([]userPurchaseData, 0)
	var total int64

	for rows.Next() {
		var p userPurchaseData
		if err := rows.Scan(&p.ID, &p.Type, &p.CreatedAt, &p.UpdatedAt, &total); err != nil {
			return nil, 0, f.fail("Error scanning purchase: ", err, "error scanning purchase")
		}
		purchases = append(purchases, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, f.fail("Error iterating purchases: ", err, "error iterating purchases")
	}

	// An empty page carries no row to read the total from, so it stays zero.
	if len(purchases) == 0 {
		return purchases, 0, nil
	}

	return purchases, total, nil
}
