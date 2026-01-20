package comments

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type GetCommentsRequest struct {
	Page     int
	Limit    int
	Email    *string
	Status   *string
	CourseID *string
	Answered *string
}

func (r *GetCommentsRequest) Validate() error {
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}

	// Normalizar status se fornecido
	if r.Status != nil && *r.Status != "" {
		status := strings.ToLower(*r.Status)
		if status != "pendent" && status != "approved" && status != "rejected" {
			// Ignorar valores inválidos, não retornar erro
			r.Status = nil
		} else {
			*r.Status = status
		}
	}

	// Normalizar answered se fornecido
	if r.Answered != nil && *r.Answered != "" {
		answered := strings.ToLower(*r.Answered)
		if answered != "true" && answered != "false" {
			// Ignorar valores inválidos, não retornar erro
			r.Answered = nil
		} else {
			*r.Answered = answered
		}
	}

	return nil
}

func ParseGetCommentsRequest(query url.Values) (*GetCommentsRequest, error) {
	req := &GetCommentsRequest{
		Page:  1,
		Limit: 10,
	}

	if pageStr := query.Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return nil, errors.New("page deve ser um número")
		}
		req.Page = page
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, errors.New("limit deve ser um número")
		}
		req.Limit = limit
	}

	if emailStr := query.Get("email"); emailStr != "" {
		req.Email = &emailStr
	}

	if statusStr := query.Get("status"); statusStr != "" {
		req.Status = &statusStr
	}

	if courseIDStr := query.Get("courseId"); courseIDStr != "" {
		req.CourseID = &courseIDStr
	}

	if answeredStr := query.Get("answered"); answeredStr != "" {
		req.Answered = &answeredStr
	}

	return req, nil
}
