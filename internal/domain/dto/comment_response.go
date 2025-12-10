package dto

import "time"

type CommentResponse struct {
	ID         string    `json:"id"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	Published  bool      `json:"published"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LessonName string    `json:"lessonName"`
	CourseName string    `json:"courseName"`
	UserName   string    `json:"username"`
	UserEmail  string    `json:"userEmail"`
}

type CommentsPaginationResponse struct {
	Data       []CommentResponse `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

func NewCommentsPaginationResponse(data []*CommentResponse, total int64, req *PaginationRequest) *CommentsPaginationResponse {
	pageSize := req.GetLimit()
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	comments := make([]CommentResponse, len(data))
	for i, c := range data {
		comments[i] = *c
	}

	return &CommentsPaginationResponse{
		Data: comments,
		Pagination: PaginationMeta{
			Page:       req.Page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
			HasNext:    req.Page < totalPages,
			HasPrev:    req.Page > 1,
		},
	}
}
