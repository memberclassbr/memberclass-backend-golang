package student

import "github.com/memberclass-backend-golang/internal/domain/dto"

type StudentRankingInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Picture *string `json:"picture"`
	Email   string  `json:"email"`
}

type StudentRankingMetrics struct {
	Logins         int     `json:"logins"`
	LessonsWatched int     `json:"lessonsWatched"`
	Comments       int     `json:"comments"`
	Ratings        int     `json:"ratings"`
	AvgRating      float64 `json:"avgRating"`
}

type StudentRankingEntry struct {
	Position int                  `json:"position"`
	Student  StudentRankingInfo   `json:"student"`
	Metrics  StudentRankingMetrics `json:"metrics"`
}

type PeriodRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type StudentsRankingResponse struct {
	Period        PeriodRange           `json:"period"`
	TotalStudents int64                 `json:"totalStudents"`
	Rankings      []StudentRankingEntry `json:"rankings"`
	Pagination    dto.PaginationMeta    `json:"pagination"`
}

type StudentRankingRow struct {
	UserID         string
	Email          string
	Name           string
	Picture        *string
	Logins         int
	LessonsWatched int
	Comments       int
	Ratings        int
	AvgRating      float64
}
