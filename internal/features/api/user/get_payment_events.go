package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lib/pq"

	"github.com/memberclass-backend-golang/internal/shared/pagination"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

// ---------- DTOs ----------

// paymentEvent is one entry of the member's payment ledger.
//
// PaymentEvent has fourteen columns and this carries ten of them. The four left
// out are not payment facts: the row's own primary key and deliveryId are
// internal identifiers, externalEventId exists so a redelivered webhook can be
// deduplicated, and createdAt records when this service received the event
// rather than when the payment happened — occurredAt is the one a reader means.
// tenantId and userId are omitted too; the caller supplied both to ask the
// question.
//
// Everything after Type is nullable because the gateway may not send it. Amount
// is in the currency's smallest unit, as gateways report it.
type paymentEvent struct {
	Type           string  `json:"type"`
	Platform       string  `json:"platform"`
	Amount         *int64  `json:"amount"`
	Currency       *string `json:"currency"`
	Plan           *string `json:"plan"`
	OccurredAt     *string `json:"occurredAt"`
	ExpiresAt      *string `json:"expiresAt"`
	DeliveryName   *string `json:"deliveryName"`
	TransactionID  *string `json:"transactionId"`
	SubscriptionID *string `json:"subscriptionId"`
}

type paymentEventsResponse struct {
	Events     []paymentEvent  `json:"events"`
	Pagination pagination.Meta `json:"pagination"`
}

// paymentEventTypes is the set the ledger records. An unknown ?type is rejected
// rather than silently returning nothing, so a typo reads as a mistake instead
// of as a member with no payments.
var paymentEventTypes = map[string]bool{
	"purchase":     true,
	"renewal":      true,
	"refund":       true,
	"chargeback":   true,
	"cancellation": true,
	"expired":      true,
}

// ---------- 1. HTTP handler ----------

// GetPaymentEvents handles `GET /api/v1/users/payment-events`: the payment
// ledger recorded for one member of the tenant.
//
// It replaces `GET /api/v1/users/purchases`, which reads the older UserEvent
// table and can only say that something happened and when. This one answers
// what was paid, on which platform, for which plan, and until when the access
// it granted runs.
func (f *Feature) GetPaymentEvents(w http.ResponseWriter, r *http.Request) {
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

	eventType := r.URL.Query().Get("type")
	if eventType != "" && !paymentEventTypes[eventType] {
		writeCustomError(w, http.StatusBadRequest,
			"type inválido. Use purchase, renewal, refund, chargeback, cancellation ou expired",
			"INVALID_EVENT_TYPE")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "Tenant not found in context")
		return
	}

	resp, err := f.getPaymentEvents(r.Context(), tenant.ID, email, eventType, page, limit)
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

func (f *Feature) getPaymentEvents(ctx context.Context, tenantID, email, eventType string, page, limit int) (*paymentEventsResponse, error) {
	userID, err := f.memberID(ctx, email, tenantID)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("payment-events:%s:%s:%d:%d:%s", tenantID, email, page, limit, eventType)

	if cached, err := f.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var hit paymentEventsResponse
		if err := json.Unmarshal([]byte(cached), &hit); err == nil {
			f.log.Debug(fmt.Sprintf("Cache hit for key: %s", cacheKey))
			return &hit, nil
		}
	}

	// No ?type means the whole ledger, refunds and cancellations included: a
	// caller asking for a member's payment history wants the reversals too.
	types := make([]string, 0, len(paymentEventTypes))
	if eventType != "" {
		types = append(types, eventType)
	} else {
		for t := range paymentEventTypes {
			types = append(types, t)
		}
	}

	events, total, err := f.queryPaymentEvents(ctx, userID, tenantID, types, page, limit)
	if err != nil {
		return nil, err
	}

	resp := &paymentEventsResponse{
		Events:     events,
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

// sqlPaymentEvents pages the member's ledger and carries the unpaginated total
// on every row, so one round trip answers both questions.
//
// The Delivery join is LEFT: deliveryId is nullable, and an event that does not
// name one — a chargeback landing before the grant resolves, say — must still
// appear rather than vanish from the history.
//
// Ordering falls back to createdAt when the gateway sent no occurredAt, so
// events without a gateway timestamp keep their place in the ledger instead of
// sinking to the end of the last page.
const sqlPaymentEvents = `
	WITH filtered AS (
		SELECT
			pe.type,
			pe.platform,
			pe.amount,
			pe.currency,
			pe.plan,
			pe."occurredAt",
			pe."expiresAt",
			pe."externalTransactionId",
			pe."externalSubscriptionId",
			pe."createdAt",
			d.name AS delivery_name
		FROM "PaymentEvent" pe
		LEFT JOIN "Delivery" d ON d.id = pe."deliveryId"
		WHERE pe."userId" = $1
		  AND pe."tenantId" = $2
		  AND pe.type = ANY($3)
	),
	paginated AS (
		SELECT * FROM filtered
		ORDER BY COALESCE("occurredAt", "createdAt") DESC
		LIMIT $4 OFFSET $5
	)
	SELECT
		p.type,
		p.platform,
		p.amount,
		p.currency,
		p.plan,
		p."occurredAt",
		p."expiresAt",
		p.delivery_name,
		p."externalTransactionId",
		p."externalSubscriptionId",
		(SELECT COUNT(*) FROM filtered) as total_count
	FROM paginated p
`

func (f *Feature) queryPaymentEvents(ctx context.Context, userID, tenantID string, types []string, page, limit int) ([]paymentEvent, int64, error) {
	rows, err := f.db.QueryContext(ctx, sqlPaymentEvents,
		userID, tenantID, pq.Array(types), limit, pagination.Offset(page, limit))
	if err != nil {
		return nil, 0, f.fail("Error finding payment events: ", err, "error finding payment events")
	}
	defer rows.Close()

	events := make([]paymentEvent, 0)
	var total int64

	for rows.Next() {
		var (
			e                             paymentEvent
			amount                        sql.NullInt64
			currency, plan, deliveryName  sql.NullString
			transactionID, subscriptionID sql.NullString
			occurredAt, expiresAt         sql.NullTime
		)

		if err := rows.Scan(
			&e.Type, &e.Platform, &amount, &currency, &plan,
			&occurredAt, &expiresAt, &deliveryName,
			&transactionID, &subscriptionID, &total,
		); err != nil {
			return nil, 0, f.fail("Error scanning payment event: ", err, "error scanning payment event")
		}

		if amount.Valid {
			e.Amount = &amount.Int64
		}
		e.Currency = stringPtr(currency)
		e.Plan = stringPtr(plan)
		e.OccurredAt = timePtr(occurredAt)
		e.ExpiresAt = timePtr(expiresAt)
		e.DeliveryName = stringPtr(deliveryName)
		e.TransactionID = stringPtr(transactionID)
		e.SubscriptionID = stringPtr(subscriptionID)

		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, f.fail("Error iterating payment events: ", err, "error iterating payment events")
	}

	// An empty page carries no row to read the total from, so it stays zero.
	if len(events) == 0 {
		return events, 0, nil
	}

	return events, total, nil
}
