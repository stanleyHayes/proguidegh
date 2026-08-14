package availability

import (
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
	cases := map[string]int{"08:00": 480, "08:30": 510, "00:00": 0, "23:59": 1439, "08:00:00": 480}
	for in, want := range cases {
		got, err := ParseClock(in)
		if err != nil {
			t.Fatalf("ParseClock(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseClock(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "8", "24:00", "08:60", "ab:cd", "25:00"} {
		if _, err := ParseClock(bad); err == nil {
			t.Fatalf("ParseClock(%q) must fail", bad)
		}
	}
	if Clock(510) != "08:30" {
		t.Fatalf("Clock(510) = %q", Clock(510))
	}
}

func TestOverlaps(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * time.Hour)
	t2 := t0.Add(4 * time.Hour)
	t3 := t0.Add(6 * time.Hour)

	if !Overlaps(t0, t2, t1, t3) {
		t.Fatal("[0,4) vs [2,6) must overlap")
	}
	if !Overlaps(t1, t3, t0, t2) {
		t.Fatal("symmetry: [2,6) vs [0,4) must overlap")
	}
	if Overlaps(t0, t1, t1, t2) {
		t.Fatal("touching endpoints must not overlap (half-open intervals)")
	}
	if Overlaps(t0, t1, t2, t3) {
		t.Fatal("disjoint intervals must not overlap")
	}
}

// mondayWindow is Monday 08:00-18:00 Africa/Accra (UTC, no DST).
func mondayWindow() Window {
	return Window{Weekday: 1, StartMin: 8 * 60, EndMin: 18 * 60, Timezone: "Africa/Accra"}
}

// 2026-06-01 is a Monday.
func monday(hour, min int) time.Time {
	return time.Date(2026, 6, 1, hour, min, 0, 0, time.UTC)
}

func TestCovered(t *testing.T) {
	w := mondayWindow()

	if !Covered([]Window{w}, monday(9, 0), monday(13, 0)) {
		t.Fatal("09:00-13:00 inside 08:00-18:00 must be covered")
	}
	if !Covered([]Window{w}, monday(8, 0), monday(18, 0)) {
		t.Fatal("exact window bounds must be covered")
	}
	if Covered([]Window{w}, monday(7, 0), monday(9, 0)) {
		t.Fatal("07:00 start is before the window; must not be covered")
	}
	if Covered([]Window{w}, monday(17, 0), monday(19, 0)) {
		t.Fatal("19:00 end is after the window; must not be covered")
	}
	if Covered(nil, monday(9, 0), monday(13, 0)) {
		t.Fatal("no windows => not covered")
	}
	if Covered([]Window{w}, monday(13, 0), monday(9, 0)) {
		t.Fatal("inverted interval => not covered")
	}

	// A booking crossing midnight needs a covering window on each day.
	tuesday := monday(0, 0).AddDate(0, 0, 1)
	if Covered([]Window{w}, monday(17, 0), tuesday.Add(1*time.Hour)) {
		t.Fatal("cross-midnight without a Tuesday window must not be covered")
	}
	fullMon := Window{Weekday: 1, StartMin: 0, EndMin: 23*60 + 59, Timezone: "Africa/Accra"}
	fullTue := Window{Weekday: 2, StartMin: 0, EndMin: 23*60 + 59, Timezone: "Africa/Accra"}
	if !Covered([]Window{fullMon, fullTue}, monday(17, 0), tuesday.Add(1*time.Hour)) {
		t.Fatal("cross-midnight with full-day windows on both days must be covered")
	}
	// The Monday segment runs to midnight: a window ending before end-of-day
	// does not cover it even with a Tuesday window present.
	if Covered([]Window{w, fullTue}, monday(17, 0), tuesday.Add(1*time.Hour)) {
		t.Fatal("Monday window ending 18:00 cannot cover a segment to midnight")
	}
}

func TestCoveredTimezone(t *testing.T) {
	// Same wall-clock window expressed in a non-UTC zone (America/New_York
	// observes DST; the check still works because coverage is evaluated in
	// the window's own zone).
	ny := Window{Weekday: 1, StartMin: 8 * 60, EndMin: 18 * 60, Timezone: "America/New_York"}
	loc, _ := time.LoadLocation("America/New_York")
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, loc)
	end := time.Date(2026, 6, 1, 13, 0, 0, 0, loc)
	if !Covered([]Window{ny}, start, end) {
		t.Fatal("09:00-13:00 New York must be covered by the 08:00-18:00 NY window")
	}
	// The same absolute times are 13:00-17:00 UTC — outside the Accra window
	// starting 08:00? No: 13-17 UTC is inside 08-18 UTC. Use a clearer case:
	// 06:00 NY = 10:00 UTC, inside Accra too. Instead verify a miss: 05:00 NY.
	early := time.Date(2026, 6, 1, 5, 0, 0, 0, loc)
	if Covered([]Window{ny}, early, start) {
		t.Fatal("05:00-09:00 NY starts before the window; must not be covered")
	}
}

func TestCoveredMultipleWindowsSameDay(t *testing.T) {
	morning := Window{Weekday: 1, StartMin: 8 * 60, EndMin: 12 * 60, Timezone: "Africa/Accra"}
	afternoon := Window{Weekday: 1, StartMin: 14 * 60, EndMin: 18 * 60, Timezone: "Africa/Accra"}
	ws := []Window{morning, afternoon}
	if !Covered(ws, monday(9, 0), monday(11, 0)) {
		t.Fatal("inside the morning window must be covered")
	}
	if Covered(ws, monday(11, 0), monday(15, 0)) {
		t.Fatal("spanning the lunch gap must not be covered")
	}
}
