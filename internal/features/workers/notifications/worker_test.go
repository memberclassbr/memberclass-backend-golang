package notifications

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

type fakeSender struct {
	mu        sync.Mutex
	topicMsgs []*messaging.Message
	multi     []*messaging.MulticastMessage

	sendErr  error
	multiErr error
	// multiResp lets the caller stage the response per call. If nil, a
	// default success-for-all response is built from the token count.
	multiResp func(*messaging.MulticastMessage) *messaging.BatchResponse
}

func (s *fakeSender) Send(_ context.Context, m *messaging.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return "", s.sendErr
	}
	s.topicMsgs = append(s.topicMsgs, m)
	return "fake-id", nil
}

func (s *fakeSender) SendEachForMulticast(_ context.Context, m *messaging.MulticastMessage) (*messaging.BatchResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.multiErr != nil {
		return nil, s.multiErr
	}
	s.multi = append(s.multi, m)
	if s.multiResp != nil {
		return s.multiResp(m), nil
	}
	resp := &messaging.BatchResponse{
		SuccessCount: len(m.Tokens),
		Responses:    make([]*messaging.SendResponse, len(m.Tokens)),
	}
	for i := range resp.Responses {
		resp.Responses[i] = &messaging.SendResponse{Success: true}
	}
	return resp, nil
}

// newTestFeature wires a Feature against a sqlmock DB and an injected fake
// fcm sender. The fcmClient is built so calling messaging() returns the
// fake without going through firebase.NewApp (which would need real creds).
func newTestFeature(t *testing.T, sender *fakeSender) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	f := New(db, fakeLogger{})
	// Pre-seed the cache so messaging() doesn't try to hit Firebase, and
	// override the factory so the cached app produces our fake.
	f.fcm = &fcmClient{
		apps: map[string]*firebase.App{"test-project": {}},
		senderFactory: func(_ context.Context, _ *firebase.App) (fcmSender, error) {
			return sender, nil
		},
	}
	return f, mock, func() { _ = db.Close() }
}

// ---------- claimPending ----------

func TestClaimPending_TransitionsToSending(t *testing.T) {
	f, mock, cleanup := newTestFeature(t, &fakeSender{})
	defer cleanup()

	rows := sqlmock.NewRows([]string{
		"id", "tenantId", "type", "fanout", "status",
		"title", "body", "messageKey", "messageData",
		"audienceType", "audienceId",
		"recipientCount", "sentCount", "failedCount", "lastBatchIndex",
		"scheduledAt", "updatedAt",
	}).AddRow(
		"n1", "t1", "ADMIN_BROADCAST", "READ", "sending",
		"Hi", "There", nil, nil,
		"tenant", nil,
		nil, 0, 0, nil,
		nil, time.Now(),
	)

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE "Notification"`)).
		WithArgs(50).
		WillReturnRows(rows)

	got, err := f.claimPending(context.Background(), 50)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "n1", got[0].ID)
	require.Equal(t, FanoutRead, got[0].Fanout)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------- dispatch: tenant audience → multicast over NotificationDevice ----------

// TestDispatch_TenantAudience_MulticastsAllDevices verifies that an
// audience=tenant broadcast enumerates NotificationDevice rows for the
// tenant (NOT a Firebase topic) and applies the anonymous-device exception
// — devices without a UsersOnTenants row always receive, regardless of
// pushDisabledTypes.
func TestDispatch_TenantAudience_MulticastsAllDevices(t *testing.T) {
	sender := &fakeSender{}
	f, mock, cleanup := newTestFeature(t, sender)
	defer cleanup()

	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY", `{"type":"service_account"}`)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "notificationsInstance" FROM "Tenant"`)).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"notificationsInstance"}).AddRow(nil))

	// Three devices: anonymous (userId=null), logged-in user-A, logged-in
	// user-B. All returned by the query because the disabled-types filter
	// was satisfied for B and bypassed for the anonymous row.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(nd."userId", ''), nd.token`)).
		WithArgs("t1", string(TypeAdminBroadcast)).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "token"}).
			AddRow("", "anon-tok").
			AddRow("uA", "tokA").
			AddRow("uB", "tokB"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "recipientCount"`)).
		WithArgs("n1", 3).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "sentCount" = $2, "failedCount" = $3, "lastBatchIndex" = $4`)).
		WithArgs("n1", 3, 0, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET status = 'sent'`)).
		WithArgs("n1", 3, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	at := string(AudienceTenant)
	n := Notification{
		ID: "n1", TenantID: "t1",
		Type: TypeAdminBroadcast, Fanout: FanoutRead,
		Title: ptr("hi"), Body: ptr("there"),
		AudienceType: &at,
	}
	require.NoError(t, f.dispatch(context.Background(), newDispatchLog(f.log, n), n))

	require.Len(t, sender.topicMsgs, 0, "must not publish to topic anymore")
	require.Len(t, sender.multi, 1)
	require.ElementsMatch(t, []string{"anon-tok", "tokA", "tokB"}, sender.multi[0].Tokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------- sendMulticast: empty recipients → marks sent 0/0 ----------

func TestSendMulticast_NoRecipients_MarksSentZero(t *testing.T) {
	sender := &fakeSender{}
	f, mock, cleanup := newTestFeature(t, sender)
	defer cleanup()

	at := string(AudienceDelivery)
	aid := "d1"
	n := Notification{
		ID: "n2", TenantID: "t1",
		Type: TypeAdminBroadcast, Fanout: FanoutRead,
		Title: ptr("hi"), Body: ptr("there"),
		AudienceType: &at, AudienceID: &aid,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT mod."memberId", nd.token`)).
		WithArgs("d1", "t1", string(TypeAdminBroadcast)).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "token"}))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "recipientCount"`)).
		WithArgs("n2", 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET status = 'sent'`)).
		WithArgs("n2", 0, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, f.sendMulticast(context.Background(), newDispatchLog(f.log, n), sender, n, "hi", "there"))
	require.Len(t, sender.multi, 0)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------- sendMulticast: resume-from-lastBatchIndex ----------

// TestSendMulticast_ResumesAfterLastBatchIndex verifies that when a row was
// already partially sent (LastBatchIndex set, SentCount > 0), the resumed
// run skips the already-finished batches, keeps the running counters, and
// only dispatches the remaining slice.
func TestSendMulticast_ResumesAfterLastBatchIndex(t *testing.T) {
	sender := &fakeSender{}
	f, mock, cleanup := newTestFeature(t, sender)
	defer cleanup()

	// Build a recipient list of 1200 tokens — 3 chunks of 500/500/200.
	const total = 1200
	rows := sqlmock.NewRows([]string{"userId", "token"})
	for i := range total {
		rows.AddRow("u", "tok-"+string(rune('a'+i%26)))
	}
	at := string(AudienceDelivery)
	aid := "d1"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT mod."memberId", nd.token`)).
		WithArgs("d1", "t1", string(TypeAdminBroadcast)).
		WillReturnRows(rows)

	// Crash happened after batch 0 (first 500 sent). LastBatchIndex=0
	// means "batch 0 was the last finished one" — resume at batch 1.
	// Two more progress UPDATEs (batches 1 and 2) and one final markSent.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "sentCount" = $2, "failedCount" = $3, "lastBatchIndex" = $4`)).
		WithArgs("nResume", 1000, 0, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "sentCount" = $2, "failedCount" = $3, "lastBatchIndex" = $4`)).
		WithArgs("nResume", 1200, 0, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET status = 'sent'`)).
		WithArgs("nResume", 1200, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rc := total
	lbi := 0
	n := Notification{
		ID: "nResume", TenantID: "t1",
		Type: TypeAdminBroadcast, Fanout: FanoutRead,
		Title: ptr("hi"), Body: ptr("there"),
		AudienceType: &at, AudienceID: &aid,
		// State as if a previous run completed batch 0 and crashed.
		SentCount:      500,
		FailedCount:    0,
		RecipientCount: &rc,
		LastBatchIndex: &lbi,
	}

	require.NoError(t, f.sendMulticast(context.Background(), newDispatchLog(f.log, n), sender, n, "hi", "there"))

	// Two batches should have been dispatched on this run, not three.
	require.Len(t, sender.multi, 2)
	require.Len(t, sender.multi[0].Tokens, 500) // batch index 1
	require.Len(t, sender.multi[1].Tokens, 200) // batch index 2 (tail)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSendMulticast_AllFailed_ReturnsError verifies that when every FCM
// response was a failure, sendMulticast returns an error so the caller
// (tick) marks the row as 'failed' instead of 'sent'.
func TestSendMulticast_AllFailed_ReturnsError(t *testing.T) {
	sender := &fakeSender{
		multiResp: func(m *messaging.MulticastMessage) *messaging.BatchResponse {
			resp := &messaging.BatchResponse{
				FailureCount: len(m.Tokens),
				Responses:    make([]*messaging.SendResponse, len(m.Tokens)),
			}
			for i := range resp.Responses {
				resp.Responses[i] = &messaging.SendResponse{Success: false}
			}
			return resp
		},
	}
	f, mock, cleanup := newTestFeature(t, sender)
	defer cleanup()

	at := string(AudienceDelivery)
	aid := "d1"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT mod."memberId", nd.token`)).
		WithArgs("d1", "t1", string(TypeAdminBroadcast)).
		WillReturnRows(sqlmock.NewRows([]string{"userId", "token"}).
			AddRow("u1", "t1").AddRow("u2", "t2"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "recipientCount"`)).
		WithArgs("nFail", 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET "sentCount" = $2, "failedCount" = $3, "lastBatchIndex" = $4`)).
		WithArgs("nFail", 0, 2, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	n := Notification{
		ID: "nFail", TenantID: "t1",
		Type: TypeAdminBroadcast, Fanout: FanoutRead,
		Title: ptr("hi"), Body: ptr("there"),
		AudienceType: &at, AudienceID: &aid,
	}

	err := f.sendMulticast(context.Background(), newDispatchLog(f.log, n), sender, n, "hi", "there")
	require.Error(t, err)
	require.Contains(t, err.Error(), "all 2 FCM sends failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------- markFailed propagates error from caller ----------

func TestMarkFailed_PersistsReason(t *testing.T) {
	f, mock, cleanup := newTestFeature(t, &fakeSender{})
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "Notification"
		SET status = 'failed'`)).
		WithArgs("n3", "boom").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, f.markFailed(context.Background(), "n3", "boom"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------- selectFirebaseKey ----------

func TestSelectFirebaseKey_DispatchesEnvByInstance(t *testing.T) {
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY_2", `{"a":1}`)
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY_3", `{"a":1}`)
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY_4", `{"a":1}`)
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY", `{"a":1}`)

	cases := []struct{ in, wantProject string }{
		{"memberclass-0825", "memberclass-0825"},
		{"mcpush3-87886", "mcpush3-87886"},
		{"mcpush4-d0d86", "mcpush4-d0d86"},
		{"", "memberclass-84a92"},
		{"custom-x", "custom-x"},
	}
	for _, c := range cases {
		_, projectID, err := selectFirebaseKey(c.in)
		require.NoError(t, err)
		require.Equal(t, c.wantProject, projectID)
	}
}

func TestSelectFirebaseKey_MissingEnv(t *testing.T) {
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY", "")
	_, _, err := selectFirebaseKey("")
	require.Error(t, err)
}

func TestSelectFirebaseKey_BadJSON(t *testing.T) {
	t.Setenv("FIREBASE_SERVICE_ACCOUNT_KEY", "not json")
	_, _, err := selectFirebaseKey("")
	require.Error(t, err)
}

