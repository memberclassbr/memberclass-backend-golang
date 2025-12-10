package entities

import "time"

type CommentPost struct {
	ID        *string    `json:"id"`
	CreatedAt *time.Time `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt"`
	Published bool       `json:"published"`
	Content   string     `json:"content"`
	UserID    *string    `json:"userId"`
	PostID    *string    `json:"postId"`
	ParentID  *string    `json:"parentId"`
}
