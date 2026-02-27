package student

import (
	"errors"
	"net/url"
	"strconv"
	"time"
)

const (
	PeriodWeek   = "week"
	PeriodMonth  = "month"
	PeriodCustom = "custom"
	MetricAll     = "all"
	MetricLogins   = "logins"
	MetricLessons  = "lessons"
	MetricComments = "comments"
	MetricRatings  = "ratings"
)

type GetStudentsRankingRequest struct {
	TenantID  string
	Period    string
	StartDate *time.Time
	EndDate   *time.Time
	Metric    string
	Limit     int
	Page      int
}

func (r *GetStudentsRankingRequest) Validate() error {
	if r.Period != PeriodWeek && r.Period != PeriodMonth && r.Period != PeriodCustom {
		return errors.New("period deve ser week, month ou custom")
	}
	if r.Period == PeriodCustom {
		if r.StartDate == nil || r.EndDate == nil {
			return errors.New("startDate e endDate são obrigatórios para período custom")
		}
		if r.StartDate.After(*r.EndDate) {
			return errors.New("startDate não pode ser maior que endDate")
		}
	}
	if r.Metric != MetricAll && r.Metric != MetricLogins && r.Metric != MetricLessons && r.Metric != MetricComments && r.Metric != MetricRatings {
		return errors.New("metric deve ser all, logins, lessons, comments ou ratings")
	}
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}
	return nil
}

func ParseStudentsRankingRequest(query url.Values) (*GetStudentsRankingRequest, error) {
	req := &GetStudentsRankingRequest{
		Period: PeriodMonth,
		Metric: MetricAll,
		Limit:  50,
		Page:   1,
	}

	if p := query.Get("period"); p != "" {
		req.Period = p
	}
	if m := query.Get("metric"); m != "" {
		req.Metric = m
	}
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, errors.New("limit deve ser um número")
		}
		req.Limit = limit
	}
	if pageStr := query.Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return nil, errors.New("page deve ser um número")
		}
		req.Page = page
	}
	if startDateStr := query.Get("startDate"); startDateStr != "" {
		t, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			return nil, errors.New("formato de data inválido para startDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)")
		}
		req.StartDate = &t
	}
	if endDateStr := query.Get("endDate"); endDateStr != "" {
		t, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			return nil, errors.New("formato de data inválido para endDate. Use ISO 8601 (YYYY-MM-DDTHH:mm:ssZ)")
		}
		req.EndDate = &t
	}

	return req, nil
}
