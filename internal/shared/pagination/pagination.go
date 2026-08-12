// Package pagination holds the paginated-response envelope shared by every
// listing endpoint. It is part of the API contract — the JSON field names here
// are what clients parse — so it lives in shared rather than being redeclared
// per slice.
package pagination

import "math"

// Meta describes the page a listing response represents.
type Meta struct {
	Page        int   `json:"page"`
	Limit       int   `json:"limit"`
	TotalCount  int64 `json:"totalCount"`
	TotalPages  int   `json:"totalPages"`
	HasNextPage bool  `json:"hasNextPage"`
	HasPrevPage bool  `json:"hasPrevPage"`
}

// NewMeta builds the envelope for a page of results. A limit of zero or less
// yields zero total pages rather than a division by zero.
func NewMeta(page, limit int, totalCount int64) Meta {
	if page < 1 {
		page = 1
	}

	totalPages := 0
	if limit > 0 {
		totalPages = int(math.Ceil(float64(totalCount) / float64(limit)))
	}

	return Meta{
		Page:        page,
		Limit:       limit,
		TotalCount:  totalCount,
		TotalPages:  totalPages,
		HasNextPage: page < totalPages,
		HasPrevPage: page > 1,
	}
}

// Offset is the SQL OFFSET for the given page.
func Offset(page, limit int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * limit
}
