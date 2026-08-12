package video

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Local fakes ----------

type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// fakeBunny records what the slice asked Bunny to do and lets a test force
// each call to fail.
type fakeBunny struct {
	collections *bunny.CollectionsResponse
	collErr     error

	createdCollection string
	createCollErr     error

	createdVideo    bunny.CreateVideoRequest
	createVideoResp *bunny.CreateVideoResponse
	createVideoErr  error

	uploaded    bunny.UploadVideoRequest
	uploadErr   error
	lastAccess  bunny.ParametersAccess
	uploadCalls int
}

func (b *fakeBunny) GetCollections(_ context.Context, access bunny.ParametersAccess) (*bunny.CollectionsResponse, error) {
	b.lastAccess = access
	return b.collections, b.collErr
}

func (b *fakeBunny) CreateCollection(_ context.Context, req bunny.CreateCollectionRequest, _ bunny.ParametersAccess) (*bunny.CreateCollectionResponse, error) {
	b.createdCollection = req.Name
	if b.createCollErr != nil {
		return nil, b.createCollErr
	}
	return &bunny.CreateCollectionResponse{GUID: "new-collection"}, nil
}

func (b *fakeBunny) CreateVideo(_ context.Context, req bunny.CreateVideoRequest, _ bunny.ParametersAccess) (*bunny.CreateVideoResponse, error) {
	b.createdVideo = req
	if b.createVideoErr != nil {
		return nil, b.createVideoErr
	}
	if b.createVideoResp != nil {
		return b.createVideoResp, nil
	}
	return &bunny.CreateVideoResponse{GUID: "video-guid"}, nil
}

func (b *fakeBunny) UploadVideo(_ context.Context, req bunny.UploadVideoRequest, _ bunny.ParametersAccess) error {
	b.uploaded = req
	b.uploadCalls++
	return b.uploadErr
}

func newTestFeature(t *testing.T) (*Feature, sqlmock.Sqlmock, *fakeBunny, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	svc := &fakeBunny{}
	return New(db, svc, fakeLogger{}), mock, svc, func() { _ = db.Close() }
}

// testUserID is the account every request in this file authenticates as.
const testUserID = "u1"

// uploadRequest builds the multipart POST the endpoint expects, already
// carrying the identity the Bearer middleware would have attached.
func uploadRequest(t *testing.T, fields map[string]string, filename string, content []byte) *http.Request {
	t.Helper()
	return anonymousUploadRequest(t, fields, filename, content).WithContext(
		middleware.ContextWithAuthUser(context.Background(), &middleware.AuthUser{
			UserID: testUserID,
			Email:  "admin@example.com",
			// Deliberately a role the endpoint would refuse if it trusted the
			// token. It does not: the role comes from the database.
			Role: "member",
		}),
	)
}

// anonymousUploadRequest is the same POST with no identity attached — what
// reaches the handler if the Bearer middleware is ever missing from the chain.
func anonymousUploadRequest(t *testing.T, fields map[string]string, filename string, content []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for k, v := range fields {
		require.NoError(t, writer.WriteField(k, v))
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/videos/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// expectRole queues the membership lookup that now runs before any Bunny work.
func expectRole(mock sqlmock.Sqlmock, tenantID, role string) {
	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs(testUserID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(role))
}

func expectCredentials(mock sqlmock.Sqlmock, tenantID, libraryID, apiKey string) {
	mock.ExpectQuery(`FROM "Tenant"`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"bunnyLibraryId", "bunnyLibraryApiKey"}).
			AddRow(libraryID, apiKey))
}

// ---------- 1. Request validation ----------

func TestUploadVideo_RequiresFile(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "", nil))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadVideo_RequiresTenantID(t *testing.T) {
	f, mock, _, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, nil, "aula.mp4", []byte("data")))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 2. Credentials ----------

// Each tenant has its own Bunny library, so the credentials must be read for
// the tenant named in the request and passed to every Bunny call.
func TestUploadVideo_UsesPerTenantCredentials(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
	svc.collections = &bunny.CollectionsResponse{
		Items: []bunny.Collection{{Name: socialCollection, GUID: "coll-1"}},
	}

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "aula.mp4", []byte("data")))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "lib-1", svc.lastAccess.LibraryID)
	assert.Equal(t, "key-1", svc.lastAccess.LibraryApiKey)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadVideo_UnknownTenant(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	expectRole(mock, "missing", "member")
	mock.ExpectQuery(`FROM "Tenant"`).WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "missing"}, "aula.mp4", []byte("data")))

	assert.Equal(t, http.StatusNotFound, w.Code)
	// Nothing was sent to Bunny.
	assert.Equal(t, 0, svc.uploadCalls)
}

// ---------- 2b. Tenant membership ----------

// Upload is open to every role, but only inside a tenant the caller belongs
// to. Without this check the Bearer would be a licence to upload into every
// tenant on the deployment.
func TestUploadVideo_RejectsTenantTheCallerDoesNotBelongTo(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	mock.ExpectQuery(`SELECT role FROM "UsersOnTenants"`).
		WithArgs(testUserID, "other-tenant").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "other-tenant"}, "aula.mp4", []byte("data")))

	assert.Equal(t, http.StatusForbidden, w.Code)
	// The Bunny credentials were never even read.
	assert.Equal(t, 0, svc.uploadCalls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Every role passes, including the most junior one.
func TestUploadVideo_AllowsEveryRole(t *testing.T) {
	for _, role := range []string{"owner", "admin", "manager", "tutor", "member"} {
		t.Run(role, func(t *testing.T) {
			f, mock, svc, done := newTestFeature(t)
			defer done()

			expectRole(mock, "t1", role)
			expectCredentials(mock, "t1", "lib-1", "key-1")
			svc.collections = &bunny.CollectionsResponse{
				Items: []bunny.Collection{{Name: socialCollection, GUID: "coll-1"}},
			}

			w := httptest.NewRecorder()
			f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "aula.mp4", []byte("data")))

			assert.Equal(t, http.StatusOK, w.Code)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// A request that reaches the handler without an identity is a 401, not a
// silent success — the guard does not depend on the middleware being present.
func TestUploadVideo_RequiresIdentity(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	w := httptest.NewRecorder()
	f.UploadVideo(w, anonymousUploadRequest(t, map[string]string{"tenantId": "t1"}, "aula.mp4", []byte("data")))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, 0, svc.uploadCalls)
	// No query ran at all.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- 3. Upload flow ----------

func TestUploadVideo_Success(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
	svc.collections = &bunny.CollectionsResponse{
		Items: []bunny.Collection{{Name: socialCollection, GUID: "coll-1"}},
	}
	svc.createVideoResp = &bunny.CreateVideoResponse{GUID: "guid-9"}

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1", "title": "Aula 1"}, "aula.mp4", []byte("payload")))

	require.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Data uploadVideoResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.True(t, envelope.Data.OK)
	assert.Equal(t, "guid-9", envelope.Data.GUID)
	assert.Equal(t, "Aula 1", envelope.Data.Title)
	assert.Equal(t, "https://iframe.mediadelivery.net/embed/lib-1/guid-9"+embedURLParams, envelope.Data.MediaURL)

	assert.Equal(t, "Aula 1", svc.createdVideo.Title)
	assert.Equal(t, "coll-1", svc.createdVideo.CollectionID)
	assert.Equal(t, []byte("payload"), svc.uploaded.File)
}

// An absent title falls back to the uploaded file's name.
func TestUploadVideo_TitleDefaultsToFilename(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
	svc.collections = &bunny.CollectionsResponse{}

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "minha-aula.mp4", []byte("data")))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "minha-aula.mp4", svc.createdVideo.Title)
}

// The collection is a convenience, not a requirement: if Bunny's collection
// API fails, the upload still goes through without one.
func TestUploadVideo_CollectionFailureDoesNotBlockUpload(t *testing.T) {
	t.Run("listing fails", func(t *testing.T) {
		f, mock, svc, done := newTestFeature(t)
		defer done()

		expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
		svc.collErr = errors.New("bunny down")

		w := httptest.NewRecorder()
		f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "a.mp4", []byte("d")))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, svc.createdVideo.CollectionID)
		assert.Equal(t, 1, svc.uploadCalls)
	})

	t.Run("creation fails", func(t *testing.T) {
		f, mock, svc, done := newTestFeature(t)
		defer done()

		expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
		svc.collections = &bunny.CollectionsResponse{}
		svc.createCollErr = errors.New("bunny down")

		w := httptest.NewRecorder()
		f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "a.mp4", []byte("d")))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, svc.createdVideo.CollectionID)
	})
}

// The social collection is created only when it is missing.
func TestUploadVideo_CreatesSocialCollectionWhenAbsent(t *testing.T) {
	f, mock, svc, done := newTestFeature(t)
	defer done()

	expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
	svc.collections = &bunny.CollectionsResponse{
		Items: []bunny.Collection{{Name: "outra", GUID: "x"}},
	}

	w := httptest.NewRecorder()
	f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "a.mp4", []byte("d")))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, socialCollection, svc.createdCollection)
	assert.Equal(t, "new-collection", svc.createdVideo.CollectionID)
}

func TestUploadVideo_BunnyFailures(t *testing.T) {
	t.Run("create video fails", func(t *testing.T) {
		f, mock, svc, done := newTestFeature(t)
		defer done()

		expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
		svc.collections = &bunny.CollectionsResponse{}
		svc.createVideoErr = errors.New("bunny rejected")

		w := httptest.NewRecorder()
		f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "a.mp4", []byte("d")))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, 0, svc.uploadCalls)
		assert.NotContains(t, w.Body.String(), "bunny rejected")
	})

	t.Run("file upload fails", func(t *testing.T) {
		f, mock, svc, done := newTestFeature(t)
		defer done()

		expectRole(mock, "t1", "member")
	expectCredentials(mock, "t1", "lib-1", "key-1")
		svc.collections = &bunny.CollectionsResponse{}
		svc.uploadErr = errors.New("bunny rejected")

		w := httptest.NewRecorder()
		f.UploadVideo(w, uploadRequest(t, map[string]string{"tenantId": "t1"}, "a.mp4", []byte("d")))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotContains(t, w.Body.String(), "bunny rejected")
	})
}

// ---------- 4. Embed URL ----------

// The stored player URL carries fixed parameters; changing them changes
// playback for content already uploaded.
func TestEmbedURL(t *testing.T) {
	got := embedURL("lib-1", "guid-9")
	assert.Equal(t,
		"https://iframe.mediadelivery.net/embed/lib-1/guid-9?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
		got)
}
