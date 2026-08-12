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

func TestResolveWindow_DefaultIsWholeCalendarDays(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	start, end := ResolveWindow(nil, nil, now, 31)

	// 2026-05-16 through 2026-06-15 inclusive is 31 calendar days.
	if want := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("start = %s, want %s", start, want)
	}
	if want := time.Date(2026, 6, 15, 23, 59, 59, 999999999, time.UTC); !end.Equal(want) {
		t.Errorf("end = %s, want %s", end, want)
	}

	// The offset-from-now form this replaced cut the oldest day at 12:00 and
	// hid anything recorded before it, so the same request answered differently
	// as the day went on.
	if start.Hour() != 0 {
		t.Errorf("oldest day starts at %02d:00, want midnight", start.Hour())
	}
}

func TestResolveWindow_DefaultIgnoresTheProcessZone(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	saoPaulo := time.FixedZone("BRT", -3*60*60)

	utcStart, utcEnd := ResolveWindow(nil, nil, now, 31)
	localStart, localEnd := ResolveWindow(nil, nil, now.In(saoPaulo), 31)

	// Parse resolves an explicit date-only bound in UTC. A default measured in
	// whatever zone the container happens to carry would cover a different day.
	if !utcStart.Equal(localStart) || !utcEnd.Equal(localEnd) {
		t.Errorf("zone changed the window: %s..%s vs %s..%s",
			utcStart, utcEnd, localStart, localEnd)
	}
}

func TestResolveWindow_StartAloneKeepsItsTimeAndClosesTheDay(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	at := time.Date(2026, 3, 10, 17, 45, 0, 0, time.UTC)

	start, end := ResolveWindow(&at, nil, now, 31)

	// Parse takes RFC3339 literally; rounding the start down here would discard
	// the time the caller spelled out.
	if !start.Equal(at) {
		t.Errorf("start = %s, want the instant as given %s", start, at)
	}
	if want := time.Date(2026, 3, 10, 23, 59, 59, 999999999, time.UTC); !end.Equal(want) {
		t.Errorf("end = %s, want %s", end, want)
	}
}

func TestResolveWindow_BareStartDateStillCoversItsDay(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	// A date-only bound arrives already at midnight, so not rounding the start
	// must not narrow it.
	at, err := Parse("2026-03-10", StartOfDay)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	start, end := ResolveWindow(&at, nil, now, 31)
	if want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("start = %s, want %s", start, want)
	}
	if want := time.Date(2026, 3, 10, 23, 59, 59, 999999999, time.UTC); !end.Equal(want) {
		t.Errorf("end = %s, want %s", end, want)
	}
}

func TestResolveWindow_BothBoundsAreUsedAsGiven(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	a := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
	b := time.Date(2026, 3, 20, 6, 0, 0, 0, time.UTC)

	start, end := ResolveWindow(&a, &b, now, 31)
	if !start.Equal(a) || !end.Equal(b) {
		t.Errorf("got %s..%s, want %s..%s", start, end, a, b)
	}
}
