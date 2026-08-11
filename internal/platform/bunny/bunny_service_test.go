package bunny

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
)

func TestBunnyService_CreateCollection(t *testing.T) {
	tests := []struct {
		name           string
		request        CreateCollectionRequest
		access         ParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedGUID   string
	}{
		{
			name: "should create collection successfully",
			request: CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"guid": "test-guid", "name": "Test Collection"}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedGUID:   "test-guid",
		},
		{
			name: "should return error when libraryID is empty",
			request: CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: ParametersAccess{
				LibraryID:     "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			request: CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"error": "Bad Request"}`,
			serverStatus:   http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/test-library/collections", r.URL.Path)
				assert.Equal(t, "test-key", r.Header.Get("AccessKey"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			service := &BunnyService{
				client:  &http.Client{},
				baseURL: server.URL + "/",
				log:     testLogger{},
			}

			result, err := service.CreateCollection(context.Background(), tt.request, tt.access)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedGUID, result.GUID)
			}
		})
	}
}

func TestBunnyService_CreateVideo(t *testing.T) {
	tests := []struct {
		name           string
		request        CreateVideoRequest
		access         ParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedGUID   string
	}{
		{
			name: "should create video successfully",
			request: CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "test-collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"guid": "test-guid", "title": "Test Video"}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedGUID:   "test-guid",
		},
		{
			name: "should return error when title is empty",
			request: CreateVideoRequest{
				Title:        "",
				CollectionID: "test-collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when collectionID is empty",
			request: CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "test-collection",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"error": "Bad Request"}`,
			serverStatus:   http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/test-library/videos", r.URL.Path)
				assert.Equal(t, "test-key", r.Header.Get("AccessKey"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			service := &BunnyService{
				client:  &http.Client{},
				baseURL: server.URL + "/",
				log:     testLogger{},
			}

			result, err := service.CreateVideo(context.Background(), tt.request, tt.access)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedGUID, result.GUID)
			}
		})
	}
}

func TestBunnyService_UploadVideo(t *testing.T) {
	tests := []struct {
		name          string
		request       UploadVideoRequest
		access        ParametersAccess
		serverStatus  int
		expectedError bool
	}{
		{
			name: "should upload video successfully",
			request: UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverStatus:  http.StatusOK,
			expectedError: false,
		},
		{
			name: "should return error when libraryID is empty",
			request: UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: ParametersAccess{
				LibraryID:     "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			request: UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverStatus:  http.StatusBadRequest,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/test-library/videos/" + tt.request.GUID
				assert.Equal(t, expectedPath, r.URL.Path)
				assert.Equal(t, "test-key", r.Header.Get("AccessKey"))
				assert.Equal(t, tt.request.ContentType, r.Header.Get("Content-Type"))

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			service := &BunnyService{
				client:  &http.Client{},
				baseURL: server.URL + "/",
				log:     testLogger{},
			}

			err := service.UploadVideo(context.Background(), tt.request, tt.access)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBunnyService_GetCollections(t *testing.T) {
	tests := []struct {
		name           string
		access         ParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedCount  int
	}{
		{
			name: "should get collections successfully",
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"items": [{"guid": "collection1"}, {"guid": "collection2"}]}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedCount:  2,
		},
		{
			name: "should return error when libraryID is empty",
			access: ParametersAccess{
				LibraryID:     "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return collections even when server returns error status",
			access: ParametersAccess{
				LibraryID:     "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"totalItems": 0, "currentPage": 0, "itemsPerPage": 0, "items": []}`,
			serverStatus:   http.StatusBadRequest,
			expectedError:  false,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/test-library/collections"
				assert.True(t, strings.HasPrefix(r.URL.Path, expectedPath))
				assert.Equal(t, "test-key", r.Header.Get("AccessKey"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			service := &BunnyService{
				client:  &http.Client{},
				baseURL: server.URL + "/",
				log:     testLogger{},
			}

			result, err := service.GetCollections(context.Background(), tt.access)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedCount, len(result.Items))
			}
		})
	}
}

// testLogger keeps these HTTP-level tests free of logging assertions.
type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}

// testConfig gives the client a base URL the tests override per case.
func testConfig() *config.Config {
	return &config.Config{Bunny: config.Bunny{BaseURL: "http://example.invalid/", Timeout: 30 * time.Second}}
}

func TestNewBunnyService(t *testing.T) {
	t.Run("should create new bunny service instance", func(t *testing.T) {
		service := NewBunnyService(testConfig(), testLogger{})

		assert.NotNil(t, service)
	})
}
