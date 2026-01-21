package comment

import (
	"math"
	"time"
)

type CommentResponse struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LessonName string    `json:"lessonName"`
	CourseName string    `json:"courseName"`
	Published  *bool     `json:"published"`
	Question   string    `json:"question"`
	Answer     *string   `json:"answer"`
	Username   string    `json:"username"`
	UserEmail  string    `json:"userEmail"`
}

type CommentsPaginationResponse struct {
	Comments   []CommentResponse      `json:"comments"`
	Pagination CommentsPaginationMeta `json:"pagination"`
}

type CommentsPaginationMeta struct {
	Page        int   `json:"page"`
	Limit       int   `json:"limit"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	HasNextPage bool  `json:"hasNextPage"`
	HasPrevPage bool  `json:"hasPrevPage"`
}

func NewCommentsPaginationResponse(data []*CommentResponse, total int64, page, limit int) *CommentsPaginationResponse {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	comments := make([]CommentResponse, len(data))
	for i, c := range data {
		comments[i] = *c
	}

	return &CommentsPaginationResponse{
		Comments: comments,
		Pagination: CommentsPaginationMeta{
			Page:        page,
			Limit:       limit,
			TotalCount:  total,
			TotalPages:  totalPages,
			HasNextPage: page < totalPages,
			HasPrevPage: page > 1,
		},
	}
}
