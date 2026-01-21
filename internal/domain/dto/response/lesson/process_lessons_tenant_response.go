package lesson

type ProcessLessonsTenantResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	JobID        *string `json:"jobId,omitempty"`
	LessonsCount int     `json:"lessonsCount"`
	TenantID     string  `json:"tenantId"`
}
