package storage

import (
	"context"
	"testing"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/stretchr/testify/assert"
)

// invalidCredentials points the client at real DigitalOcean endpoints with
// credentials that will be rejected, which is how these tests exercise the
// error paths without a live bucket.
func invalidCredentials() *config.Config {
	return &config.Config{
		Storage: config.Storage{
			AccessKey: "invalid",
			SecretKey: "invalid",
			Bucket:    "test-bucket",
			URL:       "https://sfo3.digitaloceanspaces.com",
		},
	}
}

type MockLogger struct{}

func (m *MockLogger) Info(msg string, args ...any)  {}
func (m *MockLogger) Error(msg string, args ...any) {}
func (m *MockLogger) Debug(msg string, args ...any) {}
func (m *MockLogger) Warn(msg string, args ...any)  {}

func TestExtractRegionFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"SFO3", "https://sfo3.digitaloceanspaces.com", "sfo3"},
		{"NYC3", "https://nyc3.digitaloceanspaces.com", "nyc3"},
		{"AMS3", "https://ams3.digitaloceanspaces.com", "ams3"},
		{"Invalid", "invalid-url", "nyc3"},
		{"Empty", "", "nyc3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegionFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDigitalOceanSpaces_Upload_InvalidCredentials(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	data := []byte("test data")
	filename := "test.jpg"
	contentType := "image/jpeg"

	_, err = service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

func TestDigitalOceanSpaces_Download_InvalidCredentials(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	key := "test.jpg"

	_, err = service.Download(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}

func TestDigitalOceanSpaces_Delete_InvalidCredentials(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	key := "test.jpg"

	err = service.Delete(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete file")
}

func TestDigitalOceanSpaces_Exists_InvalidCredentials(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	key := "test.jpg"

	exists, existsErr := service.Exists(ctx, key)

	assert.False(t, exists)
	if existsErr != nil {
		assert.Contains(t, existsErr.Error(), "failed to check file existence")
	}
}

func TestExtractRegionFromURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"URL with port", "https://sfo3.digitaloceanspaces.com:443", "sfo3"},
		{"URL with path", "https://sfo3.digitaloceanspaces.com/path", "sfo3"},
		{"URL with query", "https://sfo3.digitaloceanspaces.com?param=value", "sfo3"},
		{"Malformed protocol", "http://sfo3.digitaloceanspaces.com", "sfo3"},
		{"No protocol", "sfo3.digitaloceanspaces.com", "nyc3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegionFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractRegionFromURL_MoreEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"Empty host", "https://", ""},
		{"Only dots", "https://...", ""},
		{"Multiple dots", "https://sfo3.extra.digitaloceanspaces.com", "sfo3"},
		{"No dots in host", "https://sfo3", "sfo3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegionFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDigitalOceanSpaces_Upload_WithEmptyData(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	data := []byte{}
	filename := "empty.jpg"
	contentType := "image/jpeg"

	_, err = service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

func TestDigitalOceanSpaces_Download_WithURL(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	_, err = service.Download(ctx, url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}

func TestDigitalOceanSpaces_Delete_WithURL(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	err = service.Delete(ctx, url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete file")
}

func TestDigitalOceanSpaces_Exists_WithURL(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	exists, existsErr := service.Exists(ctx, url)

	assert.False(t, exists)
	if existsErr != nil {
		assert.Contains(t, existsErr.Error(), "failed to check file existence")
	}
}

func TestDigitalOceanSpaces_Exists_NotFound(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	key := "nonexistent.jpg"

	exists, _ := service.Exists(ctx, key)

	assert.False(t, exists)
}

func TestDigitalOceanSpaces_Upload_SuccessPath(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	data := []byte("test data")
	filename := "test.jpg"
	contentType := "image/jpeg"

	_, err = service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

// Reads still resolve the bucket from the object URL. Uploads no longer do:
// they always target the deployment's configured bucket.
func TestExtractBucketFromURL(t *testing.T) {
	cfg := invalidCredentials()
	cfg.Storage.Bucket = "default-bucket"
	cfg.Storage.URL = "https://nyc3.digitaloceanspaces.com"

	service, err := NewDigitalOceanSpaces(cfg, &MockLogger{})
	assert.NoError(t, err)
	dos := service.(*DigitalOceanSpaces)

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"Full DO Spaces URL", "https://my-bucket.nyc3.digitaloceanspaces.com/path/file.pdf", "my-bucket"},
		{"Different bucket", "https://other-bucket.nyc3.digitaloceanspaces.com/lessons/abc/page-1.jpg", "other-bucket"},
		{"Just a key (no URL)", "lessons/abc/page-1.jpg", "default-bucket"},
		{"Empty string", "", "default-bucket"},
		{"Non-DO URL", "https://example.com/file.pdf", "example"},
		{"URL without path", "https://my-bucket.nyc3.digitaloceanspaces.com", "my-bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dos.extractBucketFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDigitalOceanSpaces_Download_SuccessPath(t *testing.T) {
	service, err := NewDigitalOceanSpaces(invalidCredentials(), &MockLogger{})
	assert.NoError(t, err)

	ctx := context.Background()
	key := "test.jpg"

	_, err = service.Download(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}
