package comment

import "time"

type Comment struct {
	ID        *string   `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Published bool      `json:"published"`
	Question  *string   `json:"question"`
	Answer    *string   `json:"answer"`
	UserID    *string   `json:"userId"`
	LessonID  *string   `json:"lessonId"`
}
