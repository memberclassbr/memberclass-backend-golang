package datefilter

import (
	"testing"
	"time"
)

func TestParse_RFC3339IsTakenLiterally(t *testing.T) {
	// A caller who spells out the time means it. Widening this to a day
	// boundary would change the meaning of every request already in the wild.
	for _, boundary := range []Boundary{StartOfDay, EndOfDay} {
		got, err := Parse("2026-08-10T14:32:05Z", boundary)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		want := time.Date(2026, 8, 10, 14, 32, 5, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("boundary %v: got %s, want %s", boundary, got, want)
		}
	}
}

func TestParse_DateOnlyOpensAtMidnight(t *testing.T) {
	got, err := Parse("2026-08-10", StartOfDay)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParse_DateOnlyClosesAtEndOfDay(t *testing.T) {
	got, err := Parse("2026-08-13", EndOfDay)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := time.Date(2026, 8, 13, 23, 59, 59, 999999999, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

// The reason EndOfDay exists: a range given as two bare dates has to contain
// every moment of both, or it quietly returns less than it was asked for.
func TestParse_BareDateRangeCoversTheLastDay(t *testing.T) {
	start, err := Parse("2026-08-10", StartOfDay)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	end, err := Parse("2026-08-13", EndOfDay)
	if err != nil {
		t.Fatalf("end: %v", err)
	}

	lateOnTheLastDay := time.Date(2026, 8, 13, 22, 15, 0, 0, time.UTC)
	if lateOnTheLastDay.Before(start) || lateOnTheLastDay.After(end) {
		t.Errorf("%s falls outside [%s, %s] — the caller asked for the 13th and did not get it",
			lateOnTheLastDay, start, end)
	}
}

func TestParse_Rejects(t *testing.T) {
	for _, v := range []string{"", "10/08/2026", "2026-13-01", "ontem", "2026-08-10T25:00:00Z"} {
		if _, err := Parse(v, StartOfDay); err == nil {
			t.Errorf("Parse(%q) should have failed", v)
		}
	}
}
