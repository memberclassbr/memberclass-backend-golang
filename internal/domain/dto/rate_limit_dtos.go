package dto

type RateLimitRequestDTO struct {
	Key       string `json:"key"`        // userID, tenantID, IP, etc.
	FileSize  int64  `json:"file_size"`
	LimitType string `json:"limit_type"` // "upload", "api", "login", etc.
}

type RateLimitResponseDTO struct {
	Allowed        bool  `json:"allowed"`
	CurrentSize    int64 `json:"current_size"`
	MaxSize        int64 `json:"max_size"`
	RemainingSize  int64 `json:"remaining_size"`
	ResetTime      int64 `json:"reset_time"`
}

type RateLimitErrorDTO struct {
	CurrentSize    int64 `json:"current_size"`
	MaxSize        int64 `json:"max_size"`
	RemainingSize  int64 `json:"remaining_size"`
	ResetTime      int64 `json:"reset_time"`
}
