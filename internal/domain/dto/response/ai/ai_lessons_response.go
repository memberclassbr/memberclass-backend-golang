package ai

type AILessonsResponse struct {
	Lessons         []AILessonData `json:"lessons"`
	Total           int            `json:"total"`
	TenantID        string         `json:"tenantId"`
	OnlyUnprocessed bool           `json:"onlyUnprocessed"`
}

type AILessonData struct {
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
