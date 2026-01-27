package comment

import (
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
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
	Comments   []CommentResponse  `json:"comments"`
	Pagination dto.PaginationMeta `json:"pagination"`
}

func NewCommentsPaginationResponse(data []*CommentResponse, total int64, page, limit int) *CommentsPaginationResponse {
	comments := make([]CommentResponse, len(data))
	for i, c := range data {
		comments[i] = *c
	}

	paginationReq := &dto.PaginationRequest{Page: page, Limit: limit}
	return &CommentsPaginationResponse{
		Comments:   comments,
		Pagination: dto.NewPaginationMeta(total, paginationReq),
	}
}
