package request

import (
	"errors"
	"net/url"
	"strconv"
	"time"
)

type GetActivitySummaryRequest struct {
	Page      int
	Limit     int
	StartDate *time.Time
	EndDate   *time.Time
	NoAccess  bool
}

func (r *GetActivitySummaryRequest) Validate() error {
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

		diff := r.EndDate.Sub(*r.StartDate)
		if diff.Hours() > 31*24 {
			return errors.New("período máximo de 31 dias")
		}
	}

	return nil
}

func ParseActivitySummaryRequest(query url.Values) (*GetActivitySummaryRequest, error) {
	req := &GetActivitySummaryRequest{
		Page:     1,
		Limit:    10,
		NoAccess: false,
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
			return nil, errors.New("formato de data inválido para startDate")
		}
		req.StartDate = &startDate
	}

	if endDateStr := query.Get("endDate"); endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			return nil, errors.New("formato de data inválido para endDate")
		}
		req.EndDate = &endDate
	}

	if noAccessStr := query.Get("noAccess"); noAccessStr == "true" {
		req.NoAccess = true
	}

	return req, nil
}

