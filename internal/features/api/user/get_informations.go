package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/shared/pagination"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// ---------- DTOs ----------

// deliveryInfo is one access grant. Beyond the identity of the delivery it
// carries the payment lifecycle MemberOnDelivery records, so a caller can tell
// a live grant from one that expired, was refunded or was cancelled without a
// second request.
//
// Every field after Status is nullable in the schema and stays nullable here:
// expiresAt is null for a lifetime (one-off) purchase, and the platform fields
// are empty for grants created by hand rather than by a gateway webhook.
// Collapsing those to "" would make a manual grant look like a broken one.
type deliveryInfo struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	AccessDate             string  `json:"accessDate"`
	Status                 string  `json:"status"`
	ExpiresAt              *string `json:"expiresAt"`
	Platform               *string `json:"platform"`
	ExternalSubscriptionID *string `json:"externalSubscriptionId"`
	CanceledAt             *string `json:"canceledAt"`
	LastEventAt            *string `json:"lastEventAt"`
}

type userInformation struct {
	UserID     string         `json:"userId"`
	Email      string         `json:"email"`
	IsPaid     bool           `json:"isPaid"`
	Deliveries []deliveryInfo `json:"deliveries"`
	LastAccess *string        `json:"lastAccess"`
}

type userInformationsResponse struct {
	Users      []userInformation `json:"users"`
	Pagination pagination.Meta   `json:"pagination"`
}

// ---------- 1. HTTP handler ----------

// GetUserInformations handles `GET /api/v1/user/informations`. Without ?email
// it pages the tenant's whole roster; with one it returns just that member.
func (f *Feature) GetUserInformations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
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

	email := r.URL.Query().Get("email")

	resp, err := f.getInformations(r.Context(), tenant.ID, email, page, limit)
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

func (f *Feature) getInformations(ctx context.Context, tenantID, email string, page, limit int) (*userInformationsResponse, error) {
	// A filtered lookup for someone outside the tenant is a 404, not an empty
	// page — otherwise the endpoint reports "no results" for a user who exists
	// under a different tenant.
	if email != "" {
		if _, err := f.memberID(ctx, email, tenantID); err != nil {
			return nil, err
		}
	}

	// The v2 in the key is deliberate. Entries written by the previous shape
	// still unmarshal cleanly into the new one — they simply lack the delivery
	// lifecycle fields — so without a new key the endpoint would serve grants
	// with an empty status for a whole TTL after the deploy.
	cacheKey := fmt.Sprintf("user:informations:v2:%s:%s:%d:%d", tenantID, email, page, limit)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit userInformationsResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	total, err := f.countMembers(ctx, tenantID, email)
	if err != nil {
		return nil, err
	}

	users, userIDs, err := f.queryMembers(ctx, tenantID, email, page, limit)
	if err != nil {
		return nil, err
	}

	if len(userIDs) > 0 {
		byUser, err := f.queryDeliveries(ctx, userIDs, tenantID)
		if err != nil {
			return nil, err
		}
		for i := range users {
			if deliveries, ok := byUser[users[i].UserID]; ok {
				users[i].Deliveries = deliveries
			}
		}
	}

	resp := &userInformationsResponse{
		Users:      users,
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

const (
	// sqlMembers pages the tenant's members and, in the same round trip,
	// derives two flags per member: whether they ever recorded a non-negative
	// purchase event, and when they last logged in.
	//
	// last_access reads "LoginEvent", the entity that now records logins. It
	// used to read "SystemLog", the audit table, which stopped receiving logins
	// and left lastAccess null for every member. LoginEvent carries tenantId, so
	// unlike the old query this one filters by it directly rather than relying
	// on users_base having already narrowed the ids.
	//
	// $4 is the optional email filter; emailClause below wires it in.
	sqlMembers = `
		WITH users_base AS (
			SELECT
				uot."userId",
				uot."assignedAt",
				u.id,
				u.email,
				uot.name
			FROM "UsersOnTenants" uot
			JOIN "User" u ON u.id = uot."userId"
			WHERE uot."tenantId" = $1%s
			ORDER BY uot."assignedAt" DESC
			LIMIT $2 OFFSET $3
		),
		paid_users AS (
			SELECT DISTINCT "usersOnTenantsUserId" as user_id
			FROM "UserEvent"
			WHERE "usersOnTenantsUserId" IN (SELECT "userId" FROM users_base)
			  AND "usersOnTenantsTenantId" = $1
			  AND type = 'purchase'
			  AND value >= 0
		),
		last_access AS (
			SELECT DISTINCT ON (le."userId") le."userId", le."createdAt" as "updatedAt"
			FROM "LoginEvent" le
			WHERE le."userId" IN (SELECT "userId" FROM users_base)
			  AND le."tenantId" = $1
			ORDER BY le."userId", le."createdAt" DESC
		)
		SELECT
			ub."userId",
			ub.email,
			ub.name,
			(pu.user_id IS NOT NULL) as is_paid,
			la."updatedAt" as last_access
		FROM users_base ub
		LEFT JOIN paid_users pu ON pu.user_id = ub."userId"
		LEFT JOIN last_access la ON la."userId" = ub."userId"
	`

	sqlCountMembers = `
		SELECT COUNT(*)
		FROM "UsersOnTenants" uot
		JOIN "User" u ON u.id = uot."userId"
		WHERE uot."tenantId" = $1
	`

	sqlMemberDeliveries = `
		SELECT mod."memberId", mod."deliveryId", mod."assignedAt", d.name as delivery_name,
		       mod.status, mod."expiresAt", mod.platform,
		       mod."externalSubscriptionId", mod."canceledAt", mod."lastEventAt"
		FROM "MemberOnDelivery" mod
		JOIN "Delivery" d ON d.id = mod."deliveryId"
		WHERE mod."memberId" = ANY($1) AND mod."tenantId" = $2
		ORDER BY mod."assignedAt" DESC
	`
)

func (f *Feature) queryMembers(ctx context.Context, tenantID, email string, page, limit int) ([]userInformation, []string, error) {
	args := []any{tenantID, limit, pagination.Offset(page, limit)}

	emailClause := ""
	if email != "" {
		emailClause = ` AND u.email = $4`
		args = append(args, email)
	}

	rows, err := f.db.QueryContext(ctx, fmt.Sprintf(sqlMembers, emailClause), args...)
	if err != nil {
		return nil, nil, f.fail("Error finding user informations: ", err, "error finding user informations")
	}
	defer rows.Close()

	users := make([]userInformation, 0)
	userIDs := make([]string, 0)

	for rows.Next() {
		var userID, userEmail, userName string
		var isPaid bool
		var lastAccess sql.NullTime

		if err := rows.Scan(&userID, &userEmail, &userName, &isPaid, &lastAccess); err != nil {
			return nil, nil, f.fail("Error scanning user information: ", err, "error scanning user information")
		}

		users = append(users, userInformation{
			UserID:     userID,
			Email:      userEmail,
			IsPaid:     isPaid,
			Deliveries: make([]deliveryInfo, 0),
			LastAccess: timePtr(lastAccess),
		})
		userIDs = append(userIDs, userID)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, f.fail("Error iterating user informations: ", err, "error iterating user informations")
	}

	return users, userIDs, nil
}

func (f *Feature) countMembers(ctx context.Context, tenantID, email string) (int64, error) {
	query := sqlCountMembers
	args := []any{tenantID}
	if email != "" {
		query += ` AND u.email = $2`
		args = append(args, email)
	}

	var total int64
	if err := f.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, f.fail("Error counting users: ", err, "error counting users")
	}
	return total, nil
}

func (f *Feature) queryDeliveries(ctx context.Context, userIDs []string, tenantID string) (map[string][]deliveryInfo, error) {
	rows, err := f.db.QueryContext(ctx, sqlMemberDeliveries, pq.Array(userIDs), tenantID)
	if err != nil {
		return nil, f.fail("Error finding deliveries: ", err, "error finding deliveries")
	}
	defer rows.Close()

	byUser := make(map[string][]deliveryInfo)
	for rows.Next() {
		var userID, deliveryID, deliveryName, status string
		var accessDate time.Time
		var expiresAt, canceledAt, lastEventAt sql.NullTime
		var platform, externalSubscriptionID sql.NullString

		// A malformed delivery row is skipped rather than failing the page.
		if err := rows.Scan(
			&userID, &deliveryID, &accessDate, &deliveryName,
			&status, &expiresAt, &platform,
			&externalSubscriptionID, &canceledAt, &lastEventAt,
		); err != nil {
			f.log.Error("Error scanning delivery: " + err.Error())
			continue
		}

		byUser[userID] = append(byUser[userID], deliveryInfo{
			ID:                     deliveryID,
			Name:                   deliveryName,
			AccessDate:             accessDate.Format(timestampLayout),
			Status:                 status,
			ExpiresAt:              timePtr(expiresAt),
			Platform:               stringPtr(platform),
			ExternalSubscriptionID: stringPtr(externalSubscriptionID),
			CanceledAt:             timePtr(canceledAt),
			LastEventAt:            timePtr(lastEventAt),
		})
	}
	return byUser, nil
}
