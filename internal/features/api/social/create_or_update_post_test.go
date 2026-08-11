package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/shared/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return New(db, fakeLogger{}), mock, func() { _ = db.Close() }
}

func postRequest(tenantID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/social", strings.NewReader(body))
	if tenantID == "" {
		return req
	}
	ctx := context.WithValue(req.Context(), tenant.ContextKey, &tenant.Tenant{ID: tenantID})
	return req.WithContext(ctx)
}

func bodyOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func expectMembership(mock sqlmock.Sqlmock, userID, tenantID string, belongs bool) {
	mock.ExpectQuery(`FROM "UsersOnTenants"`).
		WithArgs(userID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(belongs))
}

func expectOwnerCheck(mock sqlmock.Sqlmock, userID, tenantID string, isOwner bool) {
	q := mock.ExpectQuery(`role = 'owner'`).WithArgs(userID, tenantID)
	if isOwner {
		q.WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(userID))
		return
	}
	q.WillReturnError(sql.ErrNoRows)
}

// ---------- 1. Validation ----------

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		req  createPostRequest
		want error
	}{
		{"valid create", createPostRequest{UserID: "u1", TopicID: "t1", Title: "T", Content: "C"}, nil},
		{"valid update", createPostRequest{UserID: "u1", PostID: "p1", Title: "T", Content: "C"}, nil},
		{"missing user", createPostRequest{TopicID: "t1", Title: "T", Content: "C"}, errUserIDRequired},
		{"missing topic on create", createPostRequest{UserID: "u1", Title: "T", Content: "C"}, errTopicIDRequired},
		{"missing title", createPostRequest{UserID: "u1", TopicID: "t1", Content: "C"}, errTitleRequired},
		{"missing content", createPostRequest{UserID: "u1", TopicID: "t1", Title: "T"}, errContentRequired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.validate()
			if tc.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestValidationErrorCodes(t *testing.T) {
	cases := map[string]struct {
		body     string
		wantCode string
	}{
		"missing user":    {`{"topicId":"t1","title":"T","content":"C"}`, "MISSING_USER"},
		"missing topic":   {`{"userId":"u1","title":"T","content":"C"}`, "MISSING_TOPIC"},
		"missing title":   {`{"userId":"u1","topicId":"t1","content":"C"}`, "MISSING_TITLE"},
		"missing content": {`{"userId":"u1","topicId":"t1","title":"T"}`, "MISSING_CONTENT"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, _, done := newTestFeature(t)
			defer done()

			w := httptest.NewRecorder()
			f.CreateOrUpdatePost(w, postRequest("t1", tc.body))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.wantCode, bodyOf(t, w)["errorCode"])
		})
	}
}

// ---------- 2. Tenant membership ----------

// The author id comes from the request body, so a caller holding a valid tenant
// key must not be able to post as a user outside that tenant.
func TestCreateOrUpdatePost_AuthorMustBelongToTenant(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u-other", "t1", false)

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u-other","topicId":"tp1","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "PERMISSION_DENIED", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateOrUpdatePost_RequiresTenant(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_API_KEY", bodyOf(t, w)["errorCode"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 3. Creating ----------

func TestCreatePost_Success(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u1", "t1", true)
	expectOwnerCheck(mock, "u1", "t1", false)
	mock.ExpectQuery(`FROM "Topic" t`).
		WithArgs("tp1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
			AddRow("tp1", false, "{}"))
	mock.ExpectQuery(`INSERT INTO "Post"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-post"))

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

	require.Equal(t, http.StatusOK, w.Code)
	body := bodyOf(t, w)
	assert.Equal(t, true, body["ok"])
	assert.Equal(t, "new-post", body["id"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreatePost_UnknownTopic(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u1", "t1", true)
	expectOwnerCheck(mock, "u1", "t1", false)
	mock.ExpectQuery(`FROM "Topic" t`).WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"missing","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "TOPIC_NOT_FOUND", bodyOf(t, w)["errorCode"])
}

// An admin-only topic is closed to ordinary members and open to tenant owners.
func TestCreatePost_OnlyAdminTopic(t *testing.T) {
	t.Run("member is rejected", func(t *testing.T) {
		f, mock, done := newTestFeature(t)
		defer done()

		expectMembership(mock, "u1", "t1", true)
		expectOwnerCheck(mock, "u1", "t1", false)
		mock.ExpectQuery(`FROM "Topic" t`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
				AddRow("tp1", true, "{}"))

		w := httptest.NewRecorder()
		f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "NO_ACCESS_TO_TOPIC", bodyOf(t, w)["errorCode"])
		// No INSERT was queued.
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("owner may post", func(t *testing.T) {
		f, mock, done := newTestFeature(t)
		defer done()

		expectMembership(mock, "u1", "t1", true)
		expectOwnerCheck(mock, "u1", "t1", true)
		mock.ExpectQuery(`FROM "Topic" t`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
				AddRow("tp1", true, "{}"))
		mock.ExpectQuery(`INSERT INTO "Post"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("p9"))

		w := httptest.NewRecorder()
		f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

		require.Equal(t, http.StatusOK, w.Code)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// A topic tied to deliveries only accepts members who own one of them.
func TestCreatePost_DeliveryRestrictedTopic(t *testing.T) {
	t.Run("member without a matching delivery is rejected", func(t *testing.T) {
		f, mock, done := newTestFeature(t)
		defer done()

		expectMembership(mock, "u1", "t1", true)
		expectOwnerCheck(mock, "u1", "t1", false)
		mock.ExpectQuery(`FROM "Topic" t`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
				AddRow("tp1", false, "{d1,d2}"))
		mock.ExpectQuery(`FROM "MemberOnDelivery"`).
			WithArgs("u1", "t1").
			WillReturnRows(sqlmock.NewRows([]string{"deliveryId"}).AddRow("d9"))

		w := httptest.NewRecorder()
		f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "NO_ACCESS_TO_TOPIC", bodyOf(t, w)["errorCode"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("member with a matching delivery may post", func(t *testing.T) {
		f, mock, done := newTestFeature(t)
		defer done()

		expectMembership(mock, "u1", "t1", true)
		expectOwnerCheck(mock, "u1", "t1", false)
		mock.ExpectQuery(`FROM "Topic" t`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
				AddRow("tp1", false, "{d1,d2}"))
		mock.ExpectQuery(`FROM "MemberOnDelivery"`).
			WillReturnRows(sqlmock.NewRows([]string{"deliveryId"}).AddRow("d2"))
		mock.ExpectQuery(`INSERT INTO "Post"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("p1"))

		w := httptest.NewRecorder()
		f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

		require.Equal(t, http.StatusOK, w.Code)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("owner skips the delivery check entirely", func(t *testing.T) {
		f, mock, done := newTestFeature(t)
		defer done()

		expectMembership(mock, "u1", "t1", true)
		expectOwnerCheck(mock, "u1", "t1", true)
		mock.ExpectQuery(`FROM "Topic" t`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "onlyAdmin", "deliveryIds"}).
				AddRow("tp1", false, "{d1}"))
		mock.ExpectQuery(`INSERT INTO "Post"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("p1"))

		w := httptest.NewRecorder()
		f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

		require.Equal(t, http.StatusOK, w.Code)
		// No MemberOnDelivery query was queued.
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---------- 4. Updating ----------

func TestUpdatePost_AuthorMayEditOwnPost(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u1", "t1", true)
	mock.ExpectQuery(`FROM "Post" WHERE id`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "userId"}).AddRow("p1", "u1"))
	expectOwnerCheck(mock, "u1", "t1", false)
	mock.ExpectExec(`UPDATE "Post"`).
		WithArgs("p1", "T", "C", nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","postId":"p1","title":"T","content":"C"}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "p1", bodyOf(t, w)["id"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Editing someone else's post is refused unless the caller owns the tenant.
func TestUpdatePost_OtherAuthorIsRefused(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u2", "t1", true)
	mock.ExpectQuery(`FROM "Post" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "userId"}).AddRow("p1", "u1"))
	expectOwnerCheck(mock, "u2", "t1", false)

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u2","postId":"p1","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "PERMISSION_DENIED", bodyOf(t, w)["errorCode"])
	// No UPDATE was queued.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdatePost_OwnerMayEditAnyPost(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "admin", "t1", true)
	mock.ExpectQuery(`FROM "Post" WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "userId"}).AddRow("p1", "u1"))
	expectOwnerCheck(mock, "admin", "t1", true)
	mock.ExpectExec(`UPDATE "Post"`).WillReturnResult(sqlmock.NewResult(0, 1))

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"admin","postId":"p1","title":"T","content":"C"}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdatePost_UnknownPost(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	expectMembership(mock, "u1", "t1", true)
	mock.ExpectQuery(`FROM "Post" WHERE id`).WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","postId":"missing","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "POST_NOT_FOUND", bodyOf(t, w)["errorCode"])
}

// ---------- 5. Misc ----------

func TestIntersects(t *testing.T) {
	assert.True(t, intersects([]string{"a", "b"}, []string{"b", "c"}))
	assert.False(t, intersects([]string{"a"}, []string{"b"}))
	assert.False(t, intersects(nil, []string{"a"}))
	assert.False(t, intersects([]string{"a"}, nil))
}

func TestCreateOrUpdatePost_RejectsNonPost(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, httptest.NewRequest(http.MethodGet, "/api/v1/social", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestCreateOrUpdatePost_MalformedBody(t *testing.T) {
	f, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{not json`))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrUpdatePost_DatabaseFailureIsFiveHundred(t *testing.T) {
	f, mock, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`FROM "UsersOnTenants"`).WillReturnError(errors.New("connection reset"))

	w := httptest.NewRecorder()
	f.CreateOrUpdatePost(w, postRequest("t1", `{"userId":"u1","topicId":"tp1","title":"T","content":"C"}`))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection reset")
}
