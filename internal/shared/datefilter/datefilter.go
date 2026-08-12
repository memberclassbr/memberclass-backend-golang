// Package datefilter parses the startDate / endDate query parameters shared by
// the endpoints that filter by a date window.
//
// Four slices had the same two lines of time.Parse(time.RFC3339, v) and the
// same failure: a caller who sends the obvious 2026-08-10 for a date filter
// gets a 400 telling them the format is invalid without saying what a valid one
// looks like. This package accepts both that form and full RFC3339.
//
// The date-only form needs an instant, and which instant is not a detail. Read
// as the start of its day, an endDate of 2026-08-13 excludes almost all of the
// 13th — the range silently returns less than it was asked for, which is worse
// than the error it replaced. So a date-only bound resolves to the start of the
// day when it opens a range and to the end of the day when it closes one, and
// the two together cover every day the caller named.
//
// This matches what these endpoints already do with a lone startDate: they
// expand it to cover that whole day.
package datefilter

import (
	"fmt"
	"time"
)

// dateOnlyLayout is the calendar-date form, resolved in UTC.
const dateOnlyLayout = "2006-01-02"

// Boundary says which end of a range a date-only value opens or closes.
type Boundary int

const (
	// StartOfDay resolves 2026-08-10 to 2026-08-10T00:00:00Z.
	StartOfDay Boundary = iota
	// EndOfDay resolves 2026-08-10 to 2026-08-10T23:59:59.999999999Z.
	EndOfDay
)

// startOfDay and endOfDay widen an instant to the calendar day containing it,
// keeping the instant's own location. They are unexported because StartOfDay
// and EndOfDay are already taken by the Boundary constants, which is what
// callers name at a Parse call site.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}

// ResolveWindow turns the optional startDate / endDate into a concrete range:
//
//   - neither given: the last defaultDays calendar days, ending today;
//   - startDate alone: from that instant to the end of its day;
//   - both given: used as they are.
//
// The lone-startDate case keeps the instant as given rather than rounding it
// down. A date-only bound already arrives at midnight from Parse, so it still
// covers its whole day; rounding here would instead discard the time a caller
// spelled out in RFC3339, which Parse deliberately preserved.
//
// The default is counted in whole days rather than as an offset from the
// current instant. Cutting at `now minus 31 days` left the oldest day of the
// range half covered, so a call at 14:32 could not see something recorded at
// 09:00 on the far end of the window it claimed to return, and the same request
// answered differently as the day went on.
//
// It resolves in UTC because that is the zone Parse gives a date-only bound.
// Leaving it on the process zone made the default and the explicit form measure
// days differently, on a machine whose zone nobody had chosen deliberately.
//
// Today counts as the first of the defaultDays, so the default window never
// exceeds a range a caller is allowed to ask for.
func ResolveWindow(start, end *time.Time, now time.Time, defaultDays int) (time.Time, time.Time) {
	switch {
	case start == nil && end == nil:
		today := now.UTC()
		return startOfDay(today.AddDate(0, 0, -(defaultDays - 1))), endOfDay(today)
	case start != nil && end == nil:
		return *start, endOfDay(*start)
	default:
		return *start, *end
	}
}

// Parse reads a query parameter as an instant.
//
// RFC3339 is taken literally: a caller who spells out the time means it, and
// this function will not round it to a day boundary. Only the date-only form is
// widened, according to boundary.
func Parse(value string, boundary Boundary) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}

	d, err := time.Parse(dateOnlyLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q", value)
	}

	if boundary == EndOfDay {
		return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC), nil
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), nil
}
