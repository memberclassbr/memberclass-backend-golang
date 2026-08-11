package ilovepdf

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
)

// enabledConfig is a config with iLovePDF switched on, which is the only state
// in which the constructor is expected to succeed.
func enabledConfig(keys []string, baseURL string) *config.Config {
	return &config.Config{
		IlovePDF: config.IlovePDF{Enabled: true, APIKeys: keys, BaseURL: baseURL},
	}
}

func TestNewIlovePdfService_Success(t *testing.T) {
	service, err := NewIlovePdfService(
		enabledConfig([]string{"test-key-1", "test-key-2", "test-key-3"}, "https://test.api.com"),
		fakeLogger{}, newFakeCache())

	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.IsType(t, &IlovePdfService{}, service)
}

// With no keys configured the feature is off, and building the client is an
// error rather than a client that fails on first use.
func TestNewIlovePdfService_DisabledWithoutKeys(t *testing.T) {
	service, err := NewIlovePdfService(&config.Config{}, fakeLogger{}, newFakeCache())

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "ILOVEPDF_API_KEYS")
}

// The base URL comes from config, which supplies the public API default when
// the variable is unset.
func TestNewIlovePdfService_UsesConfiguredBaseURL(t *testing.T) {
	service, err := NewIlovePdfService(
		enabledConfig([]string{"test-key-1"}, "https://api.ilovepdf.com/v1"),
		fakeLogger{}, newFakeCache())

	assert.NoError(t, err)
	assert.Equal(t, "https://api.ilovepdf.com/v1", service.(*IlovePdfService).baseURL)
}

func TestIlovePdfService_GetToken_Success(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	expectedResponse := AuthResponse{
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

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.NoError(t, err)
	assert.Equal(t, "test-token-123", token)
}

func TestIlovePdfService_GetToken_HTTPError(t *testing.T) {
	log := fakeLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestIlovePdfService_GetToken_InvalidJSON(t *testing.T) {
	log := fakeLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "failed to decode auth response")
}

func TestIlovePdfService_CreateTask_Success(t *testing.T) {
	log := fakeLogger{}

	expectedResponse := TaskResponse{
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

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	result, err := service.CreateTask("test-token")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "task-123", result.Task)
	assert.Equal(t, "server-123", result.Server)
}

func TestIlovePdfService_CreateTask_HTTPError(t *testing.T) {
	log := fakeLogger{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	result, err := service.CreateTask("test-token")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create task failed")
}

func TestIlovePdfService_ExtractImagesFromZip_Success(t *testing.T) {
	log := fakeLogger{}

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

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
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
	log := fakeLogger{}

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
	}

	images, err := service.ExtractImagesFromZip([]byte("invalid zip data"))

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "data is neither a valid JPEG image nor ZIP file")
}

func TestIlovePdfService_ExtractImagesFromZip_NoImages(t *testing.T) {
	log := fakeLogger{}

	// Create a ZIP with only non-image files
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	file, err := zipWriter.Create("document.txt")
	assert.NoError(t, err)
	file.Write([]byte("text content"))

	zipWriter.Close()

	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
	}

	images, err := service.ExtractImagesFromZip(buf.Bytes())

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "no images found in zip")
}

func TestIlovePdfService_GetToken_BlacklistedKey(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	// Mock cache to return that the key is blacklisted
	c.blacklist("test-key")

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{
				Key:      "test-key",
				LastUsed: "",
			},
		},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "all API keys are blacklisted")
}

func TestIlovePdfService_GetToken_CreditsExhaustedSingleKey(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Credits exhausted for this month"))
	}))
	defer server.Close()

	// First attempt: key not blacklisted
	// Blacklist the key
	// Second attempt: key now blacklisted, no more keys available
	c.blacklist("test-key")

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{Key: "test-key"},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "all API keys")
}

func TestIlovePdfService_IsCreditsExhaustedError(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
	}

	// Test various error messages that should be detected as credits exhausted
	testCases := []struct {
		errorMsg string
		expected bool
	}{
		{"credits exhausted", true},
		{"quota exceeded", true},
		{"authentication failed", false},
		{"network error", false},
		{"Credits exhausted for this month", true},
		{"credit limit reached", true},
		{"rate limit exceeded", true},
		{"file size limit exceeded", false},
		{"API limit reached", false},
	}

	for _, tc := range testCases {
		t.Run(tc.errorMsg, func(t *testing.T) {
			result := service.isCreditsExhaustedError(fmt.Errorf("%s", tc.errorMsg))
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIlovePdfService_BlacklistKey(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://api.ilovepdf.com/v1",
		log:     log,
		cache:   c,
	}

	// Mock cache to expect Set call with correct parameters
	// Mock logger to expect Warn call

	err := service.blacklistKey("test-key")

	assert.NoError(t, err)
}

func TestIlovePdfService_GetToken_RetryWithNextKeyOnCreditsExhausted(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)

		callCount++
		if payload["public_key"] == "exhausted-key" {
			// First key: credits exhausted
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Credits exhausted for this month"))
			return
		}

		// Second key: success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Token: "valid-token-from-key-2"})
	}))
	defer server.Close()

	// No key is blacklisted up front: the first key is blacklisted by the
	// client itself when the API reports its credits exhausted, and the retry
	// then skips it.

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{Key: "exhausted-key"},
			{Key: "good-key"},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.NoError(t, err)
	assert.Equal(t, "valid-token-from-key-2", token)
	assert.Equal(t, 2, callCount, "should have made 2 auth attempts")
}

func TestIlovePdfService_GetToken_AllKeysExhausted(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Credits exhausted for this month"))
	}))
	defer server.Close()

	// First attempt: key-1 not blacklisted

	// Second attempt: key-1 blacklisted, key-2 not blacklisted
	c.blacklist("key-1")

	// Third attempt: both blacklisted
	c.blacklist("key-1")
	c.blacklist("key-2")

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{Key: "key-1"},
			{Key: "key-2"},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "all API keys")
}

func TestIlovePdfService_GetToken_SkipsAlreadyBlacklistedKey(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)

		assert.Equal(t, "good-key", payload["public_key"], "should skip blacklisted key and use good-key directly")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{Token: "token-from-good-key"})
	}))
	defer server.Close()

	// Key 1 already blacklisted
	c.blacklist("blacklisted-key")
	// Key 2 not blacklisted

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{Key: "blacklisted-key"},
			{Key: "good-key"},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.NoError(t, err)
	assert.Equal(t, "token-from-good-key", token)
}

func TestIlovePdfService_GetToken_NonCreditErrorDoesNotRetry(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{
			{Key: "key-1"},
			{Key: "key-2"},
		},
		baseURL: server.URL,
		log:     log,
		cache:   c,
	}

	token, err := service.GetToken()

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Equal(t, 1, callCount, "should NOT retry on non-credit errors")
}

func TestIlovePdfService_IsJPEGImage(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     log,
		cache:   c,
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
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     log,
		cache:   c,
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
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     log,
		cache:   c,
	}

	// Create a simple JPEG-like data
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	images, err := service.handleSingleJPEG(jpegData)

	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.True(t, strings.HasPrefix(images[0], "data:image/jpeg;base64,"))
}

func TestIlovePdfService_ExtractImagesFromZip_SingleJPEG(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     log,
		cache:   c,
	}

	// Create JPEG data (PDF with 1 page)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	images, err := service.ExtractImagesFromZip(jpegData)

	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.True(t, strings.HasPrefix(images[0], "data:image/jpeg;base64,"))
}

func TestIlovePdfService_ExtractImagesFromZip_TooShortData(t *testing.T) {
	log := fakeLogger{}
	c := newFakeCache()

	service := &IlovePdfService{
		apiKeys: []ApiKeyInfo{},
		baseURL: "https://test.api.com",
		log:     log,
		cache:   c,
	}

	// Test with data too short
	shortData := []byte{0xFF, 0xD8}

	images, err := service.ExtractImagesFromZip(shortData)

	assert.Error(t, err)
	assert.Nil(t, images)
	assert.Contains(t, err.Error(), "data too short to be a valid file")
}
