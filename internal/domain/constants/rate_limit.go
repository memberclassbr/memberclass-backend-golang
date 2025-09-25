package constants

import "time"

const (
	// Upload Limits
	MaxUploadSizePerDay   = 10 * 1024 * 1024 * 1024
	UploadLimitExpiration = 24 * time.Hour
	UploadLimitKeyPrefix  = "rate_limit_upload_"
)
