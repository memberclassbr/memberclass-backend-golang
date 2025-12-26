package request

import (
	"errors"
	"net/url"
	"strconv"
	"time"
)

type GetStudentReportRequest struct {
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
}

func (r *GetStudentReportRequest) Validate() error {
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}

	if r.EndDate != nil && r.StartDate == nil {
		return errors.New("data de início é obrigatória quando data final é fornecida")
	}

	if r.StartDate != nil && r.EndDate != nil {
		if r.StartDate.After(*r.EndDate) {
			return errors.New("a data de início não pode ser maior que a data de fim")
		}
	}

	return nil
}

func ParseStudentReportRequest(query url.Values) (*GetStudentReportRequest, error) {
	req := &GetStudentReportRequest{
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

	if startDateStr := query.Get("startDate"); startDateStr != "" {
		startDate, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			return nil, errors.New("formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)")
		}
		req.StartDate = &startDate
	}

	if endDateStr := query.Get("endDate"); endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			return nil, errors.New("formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)")
		}
		req.EndDate = &endDate
	}

	return req, nil
}

