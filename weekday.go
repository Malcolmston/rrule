package rrule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Weekday is a BYDAY entry: a day of the week, optionally qualified by an
// ordinal. N is the ordinal occurrence of the day within the MONTHLY or
// YEARLY interval — 1 for the first, -1 for the last — and 0 for a plain,
// unqualified weekday. The ordinal is only meaningful for FREQ=MONTHLY and
// FREQ=YEARLY; for finer frequencies RFC 5545 says it must be ignored, and
// this package drops it.
type Weekday struct {
	// Day is the day of the week.
	Day time.Weekday
	// N is the ordinal occurrence, e.g. -1 for "the last one". 0 means the
	// entry is unqualified and matches every such weekday.
	N int
}

// The seven unqualified weekdays, matching the RFC 5545 BYDAY abbreviations.
// Call Nth to qualify one with an ordinal, e.g. rrule.FR.Nth(-1) is "-1FR",
// the last Friday.
var (
	// MO is Monday.
	MO = Weekday{Day: time.Monday}
	// TU is Tuesday.
	TU = Weekday{Day: time.Tuesday}
	// WE is Wednesday.
	WE = Weekday{Day: time.Wednesday}
	// TH is Thursday.
	TH = Weekday{Day: time.Thursday}
	// FR is Friday.
	FR = Weekday{Day: time.Friday}
	// SA is Saturday.
	SA = Weekday{Day: time.Saturday}
	// SU is Sunday.
	SU = Weekday{Day: time.Sunday}
)

// SundayStart selects Sunday as the week start in Options.Wkst.
//
// The zero value of Options.Wkst is time.Sunday, but an unset week start must
// mean Monday — the RFC 5545 default — so this package reads the zero value as
// "unset". Set Options.Wkst to SundayStart (or parse a rule containing
// WKST=SU) to start weeks on Sunday.
const SundayStart = time.Weekday(7)

// Nth returns a copy of w qualified by the ordinal n, so that FR.Nth(-1) is
// the RFC 5545 BYDAY value "-1FR" (the last Friday of the interval).
func (w Weekday) Nth(n int) Weekday {
	w.N = n
	return w
}

// String returns the RFC 5545 BYDAY representation of w, such as "MO",
// "2MO" or "-1FR".
func (w Weekday) String() string {
	name := weekdayNames[pyWeekday(w.Day)]
	if w.N == 0 {
		return name
	}
	return strconv.Itoa(w.N) + name
}

// weekdayNames indexes the RFC 5545 abbreviations by ISO weekday number
// (0 = Monday ... 6 = Sunday), the internal convention used throughout the
// recurrence engine.
var weekdayNames = [...]string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// pyWeekday converts a time.Weekday (Sunday = 0) to the ISO convention used
// internally (Monday = 0).
func pyWeekday(w time.Weekday) int {
	return (int(w) + 6) % 7
}

// goWeekday converts an internal ISO weekday number (Monday = 0) back to a
// time.Weekday.
func goWeekday(n int) time.Weekday {
	return time.Weekday((n + 1) % 7)
}

// parseWeekday parses one RFC 5545 BYDAY value, e.g. "MO", "+2MO" or "-1FR".
func parseWeekday(s string) (Weekday, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return Weekday{}, fmt.Errorf("%w: %q", ErrInvalidWeekday, s)
	}
	name := strings.ToUpper(s[len(s)-2:])
	idx := -1
	for i, n := range weekdayNames {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Weekday{}, fmt.Errorf("%w: %q", ErrInvalidWeekday, s)
	}
	w := Weekday{Day: goWeekday(idx)}
	if prefix := s[:len(s)-2]; prefix != "" {
		n, err := strconv.Atoi(prefix)
		if err != nil || n == 0 {
			return Weekday{}, fmt.Errorf("%w: bad ordinal in %q", ErrInvalidWeekday, s)
		}
		w.N = n
	}
	return w, nil
}
