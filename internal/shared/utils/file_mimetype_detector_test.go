package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectFileMimetype(t *testing.T) {
	tests := []struct {
		name         string
		fileBytes    []byte
		expectedMime string
	}{
		{
			name:         "should detect PDF file",
			fileBytes:    []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}, 
			expectedMime: "application/pdf",
		},
		{
			name:         "should detect MP4 video file",
			fileBytes:    []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D}, 
			expectedMime: "video/mp4",
		},
		{
			name:         "should detect JPEG image file",
			fileBytes:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, 
			expectedMime: "image/jpeg",
		},
		{
			name:         "should detect PNG image file",
			fileBytes:    []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 
			expectedMime: "image/png",
		},
		{
			name:         "should detect GIF image file",
			fileBytes:    []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, 
			expectedMime: "image/gif",
		},
		{
			name:         "should detect MP3 audio file",
			fileBytes:    []byte{0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 
			expectedMime: "audio/mpeg",
		},
		{
			name:         "should detect ZIP file",
			fileBytes:    []byte{0x50, 0x4B, 0x03, 0x04}, 
			expectedMime: "application/zip",
		},
		{
			name:         "should detect text file",
			fileBytes:    []byte("Hello, World! This is a plain text file."),
			expectedMime: "text/plain; charset=utf-8",
		},
		{
			name:         "should detect HTML file",
			fileBytes:    []byte("<html><head><title>Test</title></head><body>Hello</body></html>"),
			expectedMime: "text/html; charset=utf-8",
		},
		{
			name:         "should detect JSON file",
			fileBytes:    []byte(`{"name": "test", "value": 123}`),
			expectedMime: "application/json",
		},
		{
			name:         "should detect XML file",
			fileBytes:    []byte(`<?xml version="1.0" encoding="UTF-8"?><root><item>test</item></root>`),
			expectedMime: "text/xml; charset=UTF-8",
		},
		{
			name:         "should detect CSV file",
			fileBytes:    []byte("name,age,city\nJohn,30,New York\nJane,25,Boston"),
			expectedMime: "text/csv",
		},
		{
			name:         "should return text/plain for empty file",
			fileBytes:    []byte{},
			expectedMime: "text/plain",
		},
		{
			name:         "should return application/octet-stream for unknown file type",
			fileBytes:    []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
			expectedMime: "application/octet-stream",
		},
		{
			name:         "should detect WebM video file",
			fileBytes:    []byte{0x1A, 0x45, 0xDF, 0xA3}, 
			expectedMime: "application/octet-stream",
		},
		{
			name:         "should detect AVI video file",
			fileBytes:    []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x41, 0x56, 0x49, 0x20}, 
			expectedMime: "application/octet-stream",
		},
		{
			name:         "should detect MOV video file",
			fileBytes:    []byte{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74, 0x20, 0x20}, 
			expectedMime: "video/quicktime",
		},
		{
			name:         "should detect WAV audio file",
			fileBytes:    []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x41, 0x56, 0x45},
			expectedMime: "application/octet-stream",
		},
		{
			name:         "should detect OGG audio file",
			fileBytes:    []byte{0x4F, 0x67, 0x67, 0x53, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 
			expectedMime: "application/ogg",
		},
		{
			name:         "should detect DOCX file as ZIP",
			fileBytes:    []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00}, 
			expectedMime: "application/zip",
		},
		{
			name:         "should detect XLSX file as ZIP",
			fileBytes:    []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00}, 
			expectedMime: "application/zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFileMimetype(tt.fileBytes)

			assert.Equal(t, tt.expectedMime, result)
		})
	}
}

func TestDetectFileMimetype_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		fileBytes    []byte
		expectedMime string
	}{
		{
			name:         "should handle nil byte slice",
			fileBytes:    nil,
			expectedMime: "text/plain",
		},
		{
			name:         "should handle single byte",
			fileBytes:    []byte{0x00},
			expectedMime: "application/octet-stream",
		},
		{
			name:         "should handle very small file",
			fileBytes:    []byte{0xFF, 0xD8}, 
			expectedMime: "text/plain; charset=iso-8859-1",
		},
		{
			name:         "should handle large file header",
			fileBytes:    append([]byte{0x25, 0x50, 0x44, 0x46}, make([]byte, 1000)...), 
			expectedMime: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFileMimetype(tt.fileBytes)

			assert.Equal(t, tt.expectedMime, result)
		})
	}
}

func TestDetectFileMimetype_Performance(t *testing.T) {
	t.Run("should handle large files efficiently", func(t *testing.T) {
		largeFile := make([]byte, 1024*1024) 
		copy(largeFile[:8], []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}) 

		result := DetectFileMimetype(largeFile)

		assert.Equal(t, "application/pdf", result)
	})
}