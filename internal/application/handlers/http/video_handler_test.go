package http

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestVideoHandler_UploadVideo_Success(t *testing.T) {
	mockTenantUseCase := mocks.NewMockTenantGetTenantBunnyCredentialsUseCase(t)
	mockUploadUseCase := mocks.NewMockUploadVideoBunnyCdnUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewVideoHandler(mockTenantUseCase, mockUploadUseCase, mockLogger)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, _ := writer.CreateFormFile("file", "test.mp4")
	part.Write([]byte("fake video content"))
	writer.WriteField("tenantId", "test-tenant")
	writer.WriteField("title", "Test Video")
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("user_id", "test-user")
	req.ContentLength = int64(body.Len())

	w := httptest.NewRecorder()

	credentials := &dto.TenantBunnyCredentials{
		BunnyLibraryID:    "test-library",
		BunnyLibraryApiKey: "test-key",
	}

	uploadResponse := &dto.UploadVideoResponse{
		OK:       true,
		MediaURL: "https://example.com/video",
		GUID:     "test-guid",
		Title:    "Test Video",
	}

	mockTenantUseCase.EXPECT().
		Execute("test-tenant").
		Return(credentials, nil).
		Once()

	mockUploadUseCase.EXPECT().
		Execute(mock.Anything, mock.Anything, mock.Anything, "Test Video").
		Return(uploadResponse, nil).
		Once()

	mockLogger.EXPECT().
		Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	handler.UploadVideo(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Test Video")
}

func TestVideoHandler_UploadVideo_TenantNotFound(t *testing.T) {
	mockTenantUseCase := mocks.NewMockTenantGetTenantBunnyCredentialsUseCase(t)
	mockUploadUseCase := mocks.NewMockUploadVideoBunnyCdnUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewVideoHandler(mockTenantUseCase, mockUploadUseCase, mockLogger)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, _ := writer.CreateFormFile("file", "test.mp4")
	part.Write([]byte("fake video content"))
	writer.WriteField("tenantId", "invalid-tenant")
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("user_id", "test-user")
	req.ContentLength = int64(body.Len())

	w := httptest.NewRecorder()

	mockTenantUseCase.EXPECT().
		Execute("invalid-tenant").
		Return(nil, memberclasserrors.ErrTenantNotFound).
		Once()

	mockLogger.EXPECT().
		Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	mockLogger.EXPECT().
		Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Maybe()
	
	mockLogger.EXPECT().
		Debug(mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	handler.UploadVideo(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "tenant not found")
}

func TestVideoHandler_UploadVideo_MissingFile(t *testing.T) {
	mockTenantUseCase := mocks.NewMockTenantGetTenantBunnyCredentialsUseCase(t)
	mockUploadUseCase := mocks.NewMockUploadVideoBunnyCdnUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewVideoHandler(mockTenantUseCase, mockUploadUseCase, mockLogger)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("tenantId", "test-tenant")
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("user_id", "test-user")

	w := httptest.NewRecorder()

	mockLogger.EXPECT().
		Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Maybe()

	handler.UploadVideo(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "File is required")
}

func TestVideoHandler_UploadVideo_MissingTenantID(t *testing.T) {
	mockTenantUseCase := mocks.NewMockTenantGetTenantBunnyCredentialsUseCase(t)
	mockUploadUseCase := mocks.NewMockUploadVideoBunnyCdnUseCase(t)
	mockLogger := mocks.NewMockLogger(t)

	handler := NewVideoHandler(mockTenantUseCase, mockUploadUseCase, mockLogger)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, _ := writer.CreateFormFile("file", "test.mp4")
	part.Write([]byte("fake video content"))
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("user_id", "test-user")
	req.ContentLength = int64(body.Len())

	w := httptest.NewRecorder()

	handler.UploadVideo(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "tenantId is required")
}
