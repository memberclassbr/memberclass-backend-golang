package ilovepdf

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewIlovePdfService_Success(t *testing.T) {
	originalKeys := os.Getenv("ILOVEPDF_API_KEYS")
	originalURL := os.Getenv("ILOVEPDF_BASE_URL")
	
	defer func() {
		if originalKeys != "" {
			os.Setenv("ILOVEPDF_API_KEYS", originalKeys)
		} else {
			os.Unsetenv("ILOVEPDF_API_KEYS")
		}
		if originalURL != "" {
			os.Setenv("ILOVEPDF_BASE_URL", originalURL)
		} else {
			os.Unsetenv("ILOVEPDF_BASE_URL")
		}
	}()

	os.Setenv("ILOVEPDF_API_KEYS", "test-key-1,test-key-2,test-key-3")
	os.Setenv("ILOVEPDF_BASE_URL", "https://test.api.com")
	
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service, err := NewIlovePdfService(mockLogger, mockCache)

	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.IsType(t, &IlovePdfService{}, service)
}

func TestNewIlovePdfService_MissingAPIKey(t *testing.T) {
	originalKeys := os.Getenv("ILOVEPDF_API_KEYS")
	
	defer func() {
		if originalKeys != "" {
			os.Setenv("ILOVEPDF_API_KEYS", originalKeys)
		} else {
			os.Unsetenv("ILOVEPDF_API_KEYS")
		}
	}()

	os.Unsetenv("ILOVEPDF_API_KEYS")
	
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service, err := NewIlovePdfService(mockLogger, mockCache)

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "ILOVEPDF_API_KEYS environment variable not set")
}

func TestNewIlovePdfService_DefaultURL(t *testing.T) {
	originalKeys := os.Getenv("ILOVEPDF_API_KEYS")
	originalURL := os.Getenv("ILOVEPDF_BASE_URL")
	
	defer func() {
		if originalKeys != "" {
			os.Setenv("ILOVEPDF_API_KEYS", originalKeys)
		} else {
			os.Unsetenv("ILOVEPDF_API_KEYS")
		}
		if originalURL != "" {
			os.Setenv("ILOVEPDF_BASE_URL", originalURL)
		} else {
			os.Unsetenv("ILOVEPDF_BASE_URL")
		}
	}()

	os.Setenv("ILOVEPDF_API_KEYS", "test-key-1,test-key-2")
	os.Unsetenv("ILOVEPDF_BASE_URL")
	
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service, err := NewIlovePdfService(mockLogger, mockCache)

	assert.NoError(t, err)
	assert.NotNil(t, service)
	
	ilovePdfService := service.(*IlovePdfService)
	assert.Equal(t, "https://api.ilovepdf.com/v1", ilovePdfService.baseURL)
}

func TestIlovePdfService_GetToken_Success(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}
	
	expectedResponse := dto.AuthResponse{
		Token: "test-token-123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]string
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, "test-key", payload["public_key"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	defer server.Close()

	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	token, err := service.GetToken()

	assert.NoError(t, err)
	assert.Equal(t, "test-token-123", token)
}

func TestIlovePdfService_GetToken_HTTPError(t *testing.T) {
	mockLogger := &mocks.MockLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestIlovePdfService_GetToken_InvalidJSON(t *testing.T) {
	mockLogger := &mocks.MockLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "failed to decode auth response")
}

func TestIlovePdfService_CreateTask_Success(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	
	expectedResponse := dto.TaskResponse{
		Task:   "task-123",
		Server: "server-123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/start/pdfjpg/us", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	defer server.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	result, err := service.CreateTask("test-token")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "task-123", result.Task)
	assert.Equal(t, "server-123", result.Server)
}

func TestIlovePdfService_CreateTask_HTTPError(t *testing.T) {
	mockLogger := &mocks.MockLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	result, err := service.CreateTask("test-token")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create task failed")
}

func TestIlovePdfService_ExtractImagesFromZip_Success(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockLogger.EXPECT().Warn(mock.AnythingOfType("string")).Return()

	// Create a test ZIP with images
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add a JPG file
	file1, err := zipWriter.Create("image1.jpg")
	assert.NoError(t, err)
	file1.Write([]byte("fake jpg content 1"))

	// Add a JPEG file
	file2, err := zipWriter.Create("image2.jpeg")
	assert.NoError(t, err)
	file2.Write([]byte("fake jpeg content 2"))

	// Add a non-image file (should be ignored)
	file3, err := zipWriter.Create("document.txt")
	assert.NoError(t, err)
	file3.Write([]byte("text content"))

	zipWriter.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	images, err := service.ExtractImagesFromZip(buf.Bytes())

	assert.NoError(t, err)
	assert.Len(t, images, 2)
	
	// Check that images are base64 encoded
	assert.True(t, strings.HasPrefix(images[0], "data:image/jpeg;base64,"))
	assert.True(t, strings.HasPrefix(images[1], "data:image/jpeg;base64,"))
	
	// Decode and verify content
	base64Data1 := strings.TrimPrefix(images[0], "data:image/jpeg;base64,")
	decoded1, err := base64.StdEncoding.DecodeString(base64Data1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake jpg content 1"), decoded1)

	base64Data2 := strings.TrimPrefix(images[1], "data:image/jpeg;base64,")
	decoded2, err := base64.StdEncoding.DecodeString(base64Data2)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake jpeg content 2"), decoded2)
}

func TestIlovePdfService_ExtractImagesFromZip_InvalidZip(t *testing.T) {
	mockLogger := &mocks.MockLogger{}

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	images, err := service.ExtractImagesFromZip([]byte("invalid zip data"))

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "data is neither a valid JPEG image nor ZIP file")
}

func TestIlovePdfService_ExtractImagesFromZip_NoImages(t *testing.T) {
	mockLogger := &mocks.MockLogger{}

	// Create a ZIP with only non-image files
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	file, err := zipWriter.Create("document.txt")
	assert.NoError(t, err)
	file.Write([]byte("text content"))

	zipWriter.Close()

	mockCache := &mocks.MockCache{}
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	images, err := service.ExtractImagesFromZip(buf.Bytes())

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "no images found in zip")
}

func TestIlovePdfService_GetToken_BlacklistedKey(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	// Mock cache to return that the key is blacklisted
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(true, nil)

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "all API keys are blacklisted")
}

func TestIlovePdfService_GetToken_CreditsExhaustedError(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Credits exhausted for this month"))
	}))
	defer server.Close()

	// Mock cache to return that the key is not blacklisted initially
	mockCache.EXPECT().Exists(mock.Anything, "ilovepdf_blacklist:test-key").Return(false, nil)
	// Mock cache to set the blacklist when credits are exhausted
	mockCache.EXPECT().Set(mock.Anything, "ilovepdf_blacklist:test-key", "exhausted", mock.AnythingOfType("time.Duration")).Return(nil)
	// Mock logger to expect Warn call
	mockLogger.EXPECT().Warn(mock.AnythingOfType("string")).Return()

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     mockLogger,
		cache:   mockCache,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestIlovePdfService_IsCreditsExhaustedError(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Test various error messages that should be detected as credits exhausted
	testCases := []struct {
		errorMsg string
		expected bool
	}{
		{"credits exhausted", true},
		{"limit reached", true},
		{"quota exceeded", true},
		{"authentication failed", false},
		{"network error", false},
		{"Credits exhausted for this month", true},
		{"API limit reached", true},
	}

	for _, tc := range testCases {
		t.Run(tc.errorMsg, func(t *testing.T) {
			result := service.isCreditsExhaustedError(fmt.Errorf("%s", tc.errorMsg))
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIlovePdfService_BlacklistKey(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Mock cache to expect Set call with correct parameters
	mockCache.EXPECT().Set(
		mock.Anything,
		"ilovepdf_blacklist:test-key",
		"exhausted",
		mock.AnythingOfType("time.Duration"),
	).Return(nil)
	// Mock logger to expect Warn call
	mockLogger.EXPECT().Warn(mock.AnythingOfType("string")).Return()

	err := service.blacklistKey("test-key")

	assert.NoError(t, err)
}

func TestIlovePdfService_IsJPEGImage(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Test JPEG signature (0xFF 0xD8)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	assert.True(t, service.isJPEGImage(jpegData))

	// Test non-JPEG data
	nonJpegData := []byte{0x50, 0x4B, 0x03, 0x04} // ZIP signature
	assert.False(t, service.isJPEGImage(nonJpegData))

	// Test empty data
	emptyData := []byte{}
	assert.False(t, service.isJPEGImage(emptyData))

	// Test single byte
	singleByte := []byte{0xFF}
	assert.False(t, service.isJPEGImage(singleByte))
}

func TestIlovePdfService_IsZIPFile(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Test ZIP signature (0x50 0x4B)
	zipData := []byte{0x50, 0x4B, 0x03, 0x04}
	assert.True(t, service.isZIPFile(zipData))

	// Test non-ZIP data
	nonZipData := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG signature
	assert.False(t, service.isZIPFile(nonZipData))

	// Test empty data
	emptyData := []byte{}
	assert.False(t, service.isZIPFile(emptyData))

	// Test single byte
	singleByte := []byte{0x50}
	assert.False(t, service.isZIPFile(singleByte))
}

func TestIlovePdfService_HandleSingleJPEG(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Create a simple JPEG-like data
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	images, err := service.handleSingleJPEG(jpegData)

	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.True(t, strings.HasPrefix(images[0], "data:image/jpeg;base64,"))
}

func TestIlovePdfService_ExtractImagesFromZip_SingleJPEG(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Create JPEG data (PDF with 1 page)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	images, err := service.ExtractImagesFromZip(jpegData)

	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.True(t, strings.HasPrefix(images[0], "data:image/jpeg;base64,"))
}

func TestIlovePdfService_ExtractImagesFromZip_TooShortData(t *testing.T) {
	mockLogger := &mocks.MockLogger{}
	mockCache := &mocks.MockCache{}

	service := &IlovePdfService{
		apiKeys: []dto.ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     mockLogger,
		cache:   mockCache,
	}

	// Test with data too short
	shortData := []byte{0xFF, 0xD8}

	images, err := service.ExtractImagesFromZip(shortData)

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "data too short to be a valid file")
}

