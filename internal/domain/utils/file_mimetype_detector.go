package utils

import (
	"github.com/gabriel-vasile/mimetype"
)

func DetectFileMimetype(file []byte) string {
	detectedMimeType := mimetype.Detect(file)

	if detectedMimeType.String() == "" {
		return "application/octet-stream"
	}

	return detectedMimeType.String()
}
