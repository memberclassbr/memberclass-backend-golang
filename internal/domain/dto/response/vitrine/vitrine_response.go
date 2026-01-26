package vitrine

type VitrineResponse struct {
	Vitrines []VitrineData `json:"vitrines"`
	Total    int           `json:"total"`
}

type VitrineData struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Order   *int         `json:"order,omitempty"`
	Courses []CourseData `json:"courses,omitempty"`
}

type CourseData struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Order   *int         `json:"order,omitempty"`
	Modules []ModuleData `json:"modules,omitempty"`
}

type ModuleData struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Order   *int         `json:"order,omitempty"`
	Lessons []LessonData `json:"lessons,omitempty"`
}

type LessonData struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Slug      *string `json:"slug,omitempty"`
	Type      *string `json:"type,omitempty"`
	MediaURL  *string `json:"mediaUrl,omitempty"`
	Thumbnail *string `json:"thumbnail,omitempty"`
	Order     *int    `json:"order,omitempty"`
}

type VitrineDetailResponse struct {
	Vitrine VitrineData `json:"vitrine"`
}

type CourseDetailResponse struct {
	Course CourseData `json:"course"`
}

type ModuleDetailResponse struct {
	Module ModuleData `json:"module"`
}

type LessonDetailResponse struct {
	Lesson LessonData `json:"lesson"`
}
