package ports

import "time"

type RateLimitInfo struct {
	Limit      int
	Remaining  int
	Reset      time.Time
	RetryAfter int
}

