package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func paymentEventRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"type", "platform", "amount", "currency", "plan",
		"occurredAt", "expiresAt", "delivery_name",
		"externalTransactionId", "externalSubscriptionId", "total_count",
	})
}

func TestGetPaymentEvents_Success(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")

	occurredAt := time.Date(2026, 8, 3, 23, 32, 49, 0, time.UTC)
	expiresAt := time.Date(2027, 8, 3, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM "PaymentEvent" pe`).
		WillReturnRows(paymentEventRows().AddRow(
			"purchase", "hotmart", int64(19700), "BRL", "Anual",
			occurredAt, expiresAt, "Assinatura Anual",
			"tx_123", "sub_456", int64(1),
		))

	w := httptest.NewRecorder()
	f.GetPaymentEvents(w, requestWithTenant("/payment-events?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp paymentEventsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 1)

	e := resp.Events[0]
	assert.Equal(t, "purchase", e.Type)
	assert.Equal(t, "hotmart", e.Platform)
	require.NotNil(t, e.Amount)
	// Cents, as the gateway reports them. Dividing here would put money in a
	// float and hand the client a rounding problem it cannot see.
	assert.Equal(t, int64(19700), *e.Amount)
	require.NotNil(t, e.Currency)
	assert.Equal(t, "BRL", *e.Currency)
	require.NotNil(t, e.OccurredAt)
	assert.Equal(t, "2026-08-03T23:32:49.000Z", *e.OccurredAt)
	require.NotNil(t, e.DeliveryName)
	assert.Equal(t, "Assinatura Anual", *e.DeliveryName)
	require.NotNil(t, e.TransactionID)
	assert.Equal(t, "tx_123", *e.TransactionID)
	assert.Equal(t, int64(1), resp.Pagination.TotalCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The gateway does not always send every field. A sparse event must still be
// listed, with the missing parts null rather than zeroed.
func TestGetPaymentEvents_SparseEventKeepsNulls(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")

	mock.ExpectQuery(`FROM "PaymentEvent" pe`).
		WillReturnRows(paymentEventRows().AddRow(
			"chargeback", "hotmart", nil, nil, nil,
			nil, nil, nil, nil, nil, int64(1),
		))

	w := httptest.NewRecorder()
	f.GetPaymentEvents(w, requestWithTenant("/payment-events?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code)

	var resp paymentEventsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 1)

	e := resp.Events[0]
	assert.Equal(t, "chargeback", e.Type)
	assert.Nil(t, e.Amount, "a missing amount must not read as zero money")
	assert.Nil(t, e.Currency)
	assert.Nil(t, e.Plan)
	assert.Nil(t, e.OccurredAt)
	assert.Nil(t, e.ExpiresAt)
	assert.Nil(t, e.DeliveryName)
	assert.Nil(t, e.TransactionID)
	assert.Nil(t, e.SubscriptionID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An unknown type is a caller mistake. Passing it through to the query would
// return an empty page, which reads as "this member never paid".
func TestGetPaymentEvents_RejectsUnknownType(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetPaymentEvents(w, requestWithTenant("/payment-events?email=a@example.com&type=purchases", "t1"))

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_EVENT_TYPE", body["errorCode"])
}

func TestGetPaymentEvents_RequiresEmail(t *testing.T) {
	f, _, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.GetPaymentEvents(w, requestWithTenant("/payment-events", "t1"))

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_EMAIL", body["errorCode"])
}

// The payload must carry payment facts and nothing else. Asserting on the
// marshalled keys rather than on the SQL is deliberate: tenantId and deliveryId
// legitimately appear in the query's WHERE and JOIN, and what matters is what
// leaves the process. Once a field is in a payload it is contract, so this is
// cheaper to hold now than to withdraw later.
func TestPaymentEvent_ExposesOnlyPaymentFacts(t *testing.T) {
	amount := int64(19700)
	s := "x"
	encoded, err := json.Marshal(paymentEvent{
		Type: "purchase", Platform: "hotmart", Amount: &amount,
		Currency: &s, Plan: &s, OccurredAt: &s, ExpiresAt: &s,
		DeliveryName: &s, TransactionID: &s, SubscriptionID: &s,
	})
	require.NoError(t, err)

	var keys map[string]any
	require.NoError(t, json.Unmarshal(encoded, &keys))

	want := []string{
		"type", "platform", "amount", "currency", "plan",
		"occurredAt", "expiresAt", "deliveryName",
		"transactionId", "subscriptionId",
	}
	got := make([]string, 0, len(keys))
	for k := range keys {
		got = append(got, k)
	}
	assert.ElementsMatch(t, want, got,
		"the ledger's row id, tenantId, deliveryId, externalEventId and the "+
			"webhook's own createdAt are not payment facts and must stay out")
}

// A dedicated database per customer is isolation, not a reason to drop the
// tenant filter. If a deployment is ever pointed at the wrong database, this
// clause is what stops one customer reading another's payments.
func TestPaymentEventsSQL_KeepsTenantScope(t *testing.T) {
	assert.Contains(t, sqlPaymentEvents, `pe."tenantId" = $2`)
	assert.Contains(t, sqlPaymentEvents, `pe."userId" = $1`)
}

// The deprecated endpoint has to keep answering, and has to say where to go.
func TestGetUserPurchases_AdvertisesItsSuccessor(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	expectMemberLookup(mock, "a@example.com", "t1", "u1")
	mock.ExpectQuery(`WITH filtered AS`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "createdAt", "updatedAt", "total_count"}))

	w := httptest.NewRecorder()
	f.GetUserPurchases(w, requestWithTenant("/purchases?email=a@example.com", "t1"))

	require.Equal(t, http.StatusOK, w.Code, "deprecated is not removed: existing callers must keep working")
	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.True(t, strings.Contains(w.Header().Get("Link"), successorPath),
		"Link header should name %s, got %q", successorPath, w.Header().Get("Link"))
}
