package bunny

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBunnyService_CreateCollection(t *testing.T) {
	tests := []struct {
		name           string
		request        dto.CreateCollectionRequest
		access         dto.BunnyParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedGUID   string
	}{
		{
			name: "should create collection successfully",
			request: dto.CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"guid": "test-guid", "name": "Test Collection"}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedGUID:   "test-guid",
		},
		{
			name: "should return error when libraryID is empty",
			request: dto.CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			request: dto.CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: dto.CreateCollectionRequest{
				Name: "Test Collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"error": "Bad Request"}`,
			serverStatus:   http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			
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
				log:     mockLogger,
			}

			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

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
		request        dto.CreateVideoRequest
		access         dto.BunnyParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedGUID   string
	}{
		{
			name: "should create video successfully",
			request: dto.CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "test-collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"guid": "test-guid", "title": "Test Video"}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedGUID:   "test-guid",
		},
		{
			name: "should return error when title is empty",
			request: dto.CreateVideoRequest{
				Title:        "",
				CollectionID: "test-collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when collectionID is empty",
			request: dto.CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: dto.CreateVideoRequest{
				Title:        "Test Video",
				CollectionID: "test-collection",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"error": "Bad Request"}`,
			serverStatus:   http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			
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
				log:     mockLogger,
			}

			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

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
		request       dto.UploadVideoRequest
		access        dto.BunnyParametersAccess
		serverStatus  int
		expectedError bool
	}{
		{
			name: "should upload video successfully",
			request: dto.UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverStatus:  http.StatusOK,
			expectedError: false,
		},
		{
			name: "should return error when libraryID is empty",
			request: dto.UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			request: dto.UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return error when server returns error",
			request: dto.UploadVideoRequest{
				GUID:        "test-guid",
				File:        []byte("fake video content"),
				ContentType: "video/mp4",
			},
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverStatus:  http.StatusBadRequest,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)
			
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
				log:     mockLogger,
			}

			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

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
		access         dto.BunnyParametersAccess
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedCount  int
	}{
		{
			name: "should get collections successfully",
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			serverResponse: `{"items": [{"guid": "collection1"}, {"guid": "collection2"}]}`,
			serverStatus:   http.StatusOK,
			expectedError:  false,
			expectedCount:  2,
		},
		{
			name: "should return error when libraryID is empty",
			access: dto.BunnyParametersAccess{
				LibraryID:    "",
				LibraryApiKey: "test-key",
			},
			expectedError: true,
		},
		{
			name: "should return error when libraryApiKey is empty",
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "",
			},
			expectedError: true,
		},
		{
			name: "should return collections even when server returns error status",
			access: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
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
			mockLogger := mocks.NewMockLogger(t)
			
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
				log:     mockLogger,
			}

			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
			mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

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

func TestNewBunnyService(t *testing.T) {
	t.Run("should create new bunny service instance", func(t *testing.T) {
		mockLogger := mocks.NewMockLogger(t)
		
		mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
		
		service := NewBunnyService(mockLogger)
		
		assert.NotNil(t, service)
	})
}