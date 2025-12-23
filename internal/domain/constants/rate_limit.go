package constants

import "time"

const (
	MaxUploadSizePerDay   = 10 * 1024 * 1024 * 1024
	UploadLimitExpiration = 24 * time.Hour
	UploadLimitKeyPrefix  = "rate_limit_upload_"

	APIRateLimitTenantKeyPrefix = "rate_limit_api_tenant_"
	APIRateLimitIPKeyPrefix     = "rate_limit_api_ip_"

	APIRateLimitTenantLimit   = 50
	APIRateLimitIPLimit       = 50
	APIRateLimitWindow        = 1 * time.Minute
	APIRateLimitWindowSeconds = 60
)
