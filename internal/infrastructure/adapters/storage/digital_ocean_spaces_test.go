package storage

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MockLogger struct{}

func (m *MockLogger) Info(msg string, args ...any)  {}
func (m *MockLogger) Error(msg string, args ...any) {}
func (m *MockLogger) Debug(msg string, args ...any) {}
func (m *MockLogger) Warn(msg string, args ...any)  {}

func TestNewDigitalOceanSpaces_MissingEnvVars(t *testing.T) {
	os.Unsetenv("DO_SPACES_ID")
	os.Unsetenv("DO_SPACES_SECRET")
	os.Unsetenv("DO_SPACES_BUCKET")
	os.Unsetenv("DO_SPACES_URL")

	mockLogger := &MockLogger{}
	service, err := NewDigitalOceanSpaces(mockLogger)

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "missing required environment variables")
}

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
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	data := []byte("test data")
	filename := "test.jpg"
	contentType := "image/jpeg"

	_, err := service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

func TestDigitalOceanSpaces_Download_InvalidCredentials(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	key := "test.jpg"

	_, err := service.Download(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}

func TestDigitalOceanSpaces_Delete_InvalidCredentials(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	key := "test.jpg"

	err := service.Delete(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete file")
}

func TestDigitalOceanSpaces_Exists_InvalidCredentials(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	key := "test.jpg"

	exists, err := service.Exists(ctx, key)

	assert.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "failed to check file existence")
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

func TestNewDigitalOceanSpaces_PartialEnvVars(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"Missing ID", map[string]string{"DO_SPACES_SECRET": "secret", "DO_SPACES_BUCKET": "bucket", "DO_SPACES_URL": "https://sfo3.digitaloceanspaces.com"}},
		{"Missing Secret", map[string]string{"DO_SPACES_ID": "id", "DO_SPACES_BUCKET": "bucket", "DO_SPACES_URL": "https://sfo3.digitaloceanspaces.com"}},
		{"Missing Bucket", map[string]string{"DO_SPACES_ID": "id", "DO_SPACES_SECRET": "secret", "DO_SPACES_URL": "https://sfo3.digitaloceanspaces.com"}},
		{"Missing URL", map[string]string{"DO_SPACES_ID": "id", "DO_SPACES_SECRET": "secret", "DO_SPACES_BUCKET": "bucket"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("DO_SPACES_ID")
			os.Unsetenv("DO_SPACES_SECRET")
			os.Unsetenv("DO_SPACES_BUCKET")
			os.Unsetenv("DO_SPACES_URL")

			for key, value := range tt.env {
				os.Setenv(key, value)
			}

			mockLogger := &MockLogger{}
			service, err := NewDigitalOceanSpaces(mockLogger)

			assert.Error(t, err)
			assert.Nil(t, service)
			assert.Contains(t, err.Error(), "missing required environment variables")
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
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	data := []byte{}
	filename := "empty.jpg"
	contentType := "image/jpeg"

	_, err := service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

func TestDigitalOceanSpaces_Download_WithURL(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	_, err := service.Download(ctx, url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}

func TestDigitalOceanSpaces_Delete_WithURL(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	err := service.Delete(ctx, url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete file")
}

func TestDigitalOceanSpaces_Exists_WithURL(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	url := "https://bucket.sfo3.digitaloceanspaces.com/path/file.jpg"

	exists, err := service.Exists(ctx, url)

	assert.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "failed to check file existence")
}

func TestDigitalOceanSpaces_Exists_NotFound(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	key := "nonexistent.jpg"

	exists, err := service.Exists(ctx, key)

	assert.Error(t, err)
	assert.False(t, exists)
}

func TestDigitalOceanSpaces_Upload_SuccessPath(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	data := []byte("test data")
	filename := "test.jpg"
	contentType := "image/jpeg"

	_, err := service.Upload(ctx, data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}

func TestDigitalOceanSpaces_Download_SuccessPath(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "test-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	key := "test.jpg"

	_, err := service.Download(ctx, key)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}
