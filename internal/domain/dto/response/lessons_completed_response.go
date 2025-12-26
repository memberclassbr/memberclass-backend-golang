package response

type CompletedLesson struct {
	CourseName  string `json:"courseName"`
	LessonName  string `json:"lessonName"`
	CompletedAt string `json:"completedAt"`
}

type LessonsCompletedData struct {
	CompletedLessons []CompletedLesson `json:"completedLessons"`
	Pagination       Pagination         `json:"pagination"`
}

type LessonsCompletedResponse struct {
	OK   bool                  `json:"ok"`
	Data LessonsCompletedData `json:"data"`
}

