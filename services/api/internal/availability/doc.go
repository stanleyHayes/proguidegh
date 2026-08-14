// Package availability implements guide availability (spec §10.1–10.2,
// Phase 3): the recurring weekly schedule (guide_availability), one-off
// time off (guide_time_off) and the Redis-backed online presence state.
//
// The pure coverage/overlap logic lives here (unit-tested, no database);
// the repository owns explicit SQL for the two tables, and Presence owns
// the ephemeral online flag (ADR 0003: Redis holds ephemeral state only).
package availability

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is one recurring weekly availability window: a guide works every
// Weekday (0 = Sunday, matching Postgres extract(dow)) between StartMin and
// EndMin minutes past midnight in Timezone.
type Window struct {
	Weekday  int
	StartMin int
	EndMin   int
	Timezone string
}

// ParseClock parses an "HH:MM" (or "HH:MM:SS") wall-clock time into minutes
// past midnight. Rejects 24:00+ — overnight windows are not supported in V1
// (split them into two rows at midnight).
func ParseClock(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("availability: bad clock %q, want HH:MM", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("availability: bad clock %q: %w", s, err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("availability: bad clock %q: %w", s, err)
	}
	sec := 0
	if len(parts) == 3 {
		sec, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("availability: bad clock %q: %w", s, err)
		}
	}
	if h < 0 || h > 23 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, fmt.Errorf("availability: clock %q out of range", s)
	}
	return h*60 + m, nil
}

// Clock renders minutes past midnight as "HH:MM".
func Clock(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// BookingActiveStatuses are the on-calendar booking statuses: a booking in
// one of these blocks the guide's schedule (overlap guard, spec §10.2). This
// is the single definition shared by the bookings package (creation and
// transitions), the search exclusion filter and migration 0004
// (bookings_no_guide_overlap + idx_bookings_guide_active must stay in sync).
// It lives here, not in bookings, so search can use it without an import
// cycle (bookings imports this package).
var BookingActiveStatuses = []string{
	"CONFIRMED", "GUIDE_EN_ROUTE", "GUIDE_ARRIVED", "IN_PROGRESS",
}

// Overlaps reports whether half-open intervals [aStart,aEnd) and [bStart,bEnd)
// intersect. Touching endpoints do not overlap.
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

// Covered reports whether the whole interval [start, end) fits inside the
// guide's recurring weekly windows. The interval is split at local midnight
// (in each window's timezone); every day-segment must be fully contained in
// one window of that weekday — a booking may not span a gap between two
// windows or cross a weekday boundary into an unscheduled day.
//
// V1 simplifications (documented, spec §10.1): coverage is evaluated in the
// schedule's timezone, which is assumed to have no DST transitions
// (Africa/Accra — UTC — is the default); windows may not cross midnight, so a
// multi-day package needs a covering window on every local day it touches.
func Covered(windows []Window, start, end time.Time) bool {
	if !start.Before(end) || len(windows) == 0 {
		return false
	}
	byTZ := map[string][]Window{}
	for _, w := range windows {
		byTZ[w.Timezone] = append(byTZ[w.Timezone], w)
	}
	for tz, ws := range byTZ {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}
		if coveredIn(ws, loc, start, end) {
			return true
		}
	}
	return false
}

func coveredIn(ws []Window, loc *time.Location, start, end time.Time) bool {
	cursor := start
	for cursor.Before(end) {
		c := cursor.In(loc)
		dayStart := time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, loc)
		nextDay := dayStart.AddDate(0, 0, 1)
		segEnd := end
		if nextDay.Before(end) {
			segEnd = nextDay
		}
		segStartMin := minutesPastMidnight(cursor.In(loc))
		segEndMin := 24 * 60
		if !segEnd.Equal(nextDay) {
			segEndMin = ceilMinutes(segEnd.In(loc))
		}
		weekday := int(dayStart.Weekday())
		covered := false
		for _, w := range ws {
			if w.Weekday != weekday || w.StartMin > segStartMin {
				continue
			}
			// A window ending at 23:59 means "to end of day": it contains a
			// segment that runs into midnight (end-of-day can't be expressed
			// as a wall-clock time otherwise).
			if segEndMin <= w.EndMin || (segEndMin == 24*60 && w.EndMin >= 23*60+59) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
		cursor = segEnd
	}
	return true
}

func minutesPastMidnight(t time.Time) int { return t.Hour()*60 + t.Minute() }

// ceilMinutes rounds up to the next minute when seconds are present, so a
// window ending on the minute is not wrongly treated as containing a segment
// that ends a few seconds past it.
func ceilMinutes(t time.Time) int {
	m := minutesPastMidnight(t)
	if t.Second() > 0 || t.Nanosecond() > 0 {
		m++
	}
	return m
}
