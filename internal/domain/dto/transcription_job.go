package dto

type TranscriptionJobRequest struct {
	Lessons  []TranscriptionLessonData `json:"lessons"`
	TenantID string                    `json:"tenantId"`
}

type TranscriptionLessonData struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	Slug                   string  `json:"slug"`
	Type                   *string `json:"type"`
	MediaURL               *string `json:"mediaUrl"`
	Thumbnail              *string `json:"thumbnail"`
	Content                *string `json:"content"`
	TranscriptionCompleted bool    `json:"transcriptionCompleted"`
	ModuleID               string  `json:"moduleId"`
	ModuleName             string  `json:"moduleName"`
	SectionID              string  `json:"sectionId"`
	SectionName            string  `json:"sectionName"`
	CourseID               string  `json:"courseId"`
	CourseName             string  `json:"courseName"`
	VitrineID              string  `json:"vitrineId"`
	VitrineName            string  `json:"vitrineName"`
}

type TranscriptionJobResponse struct {
	JobID      string   `json:"jobId"`
	Status     string   `json:"status"`
	VideoIDs   []string `json:"videoIds"`
	QueuedJobs int      `json:"queuedJobs"`
	TraceID    string   `json:"traceId"`
}

type TranscriptionJobStatusResponse struct {
	JobID       string                      `json:"jobId"`
	Status      string                      `json:"status"`
	Progress    int                         `json:"progress"`
	Completed   int                         `json:"completed"`
	Failed      int                         `json:"failed"`
	Total       int                         `json:"total"`
	StartedAt   string                      `json:"startedAt"`
	CompletedAt *string                     `json:"completedAt"`
	Lessons     []TranscriptionLessonStatus `json:"lessons"`
}

type TranscriptionLessonStatus struct {
	ID               string  `json:"id"`
	LessonID         string  `json:"lessonId"`
	LessonName       string  `json:"lessonName"`
	Status           string  `json:"status"`
	ChunksCreated    *int    `json:"chunksCreated"`
	ProcessingTimeMs *int64  `json:"processingTimeMs"`
	ErrorMessage     *string `json:"errorMessage"`
}

type TranscriptionJobData struct {
	JobID     string   `json:"jobId"`
	TenantID  string   `json:"tenantId"`
	LessonIDs []string `json:"lessonIds"`
	CreatedAt string   `json:"createdAt"`
}

