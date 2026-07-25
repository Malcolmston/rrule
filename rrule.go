package rrule

import (
	"fmt"
	"sort"
	"time"
)

// MaxAllOccurrences bounds All on a rule that has neither COUNT nor UNTIL.
// Such a rule is infinite, so All stops after this many occurrences instead of
// never returning. Use Between, After or Iterator to walk an unbounded rule
// under your own control.
const MaxAllOccurrences = 10000

// maxYear is the highest calendar year the engine will iterate into. It
// matches Python datetime.MAXYEAR and guarantees termination of every loop.
const maxYear = 9999

// maxEmptyIterations bounds how many consecutive frequency intervals may
// produce no occurrence before iteration gives up. It is what stops an
// impossible rule such as FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2 from spinning
// forever, while remaining generous enough for legitimately sparse rules such
// as FREQ=DAILY;BYMONTH=2;BYMONTHDAY=29 (up to eight years, ~2920 empty days,
// between occurrences).
const maxEmptyIterations = 10000

// Options describes a recurrence rule. It mirrors the RFC 5545 RRULE rule
// parts one for one; the zero value of a BYxxx slice means the rule part is
// absent, not empty.
//
// DTStart anchors the recurrence: its date supplies the defaults for the
// BYxxx parts that FREQ leaves unspecified, its time of day supplies the
// defaults for BYHOUR, BYMINUTE and BYSECOND, and its time.Location is the
// location every occurrence is generated in. A zero DTStart defaults to the
// current local time truncated to the second.
type Options struct {
	// Freq is the base frequency (required).
	Freq Freq
	// DTStart is the first candidate instant and the location of the rule.
	DTStart time.Time
	// Interval is the number of frequency units between occurrences. Zero
	// means 1.
	Interval int
	// Wkst is the day a week starts on, affecting BYWEEKNO and the boundaries
	// of a FREQ=WEEKLY interval. The zero value means the RFC 5545 default,
	// Monday; use SundayStart to select Sunday.
	Wkst time.Weekday
	// Count limits the rule to this many occurrences. Zero means unlimited.
	Count int
	// Until is the inclusive upper bound of the recurrence. The zero value
	// means unbounded.
	Until time.Time
	// BySetPos selects occurrences by position within each interval, counting
	// from the end when negative. It is only meaningful together with another
	// BYxxx rule part.
	BySetPos []int
	// ByMonth limits occurrences to these months, 1-12.
	ByMonth []int
	// ByMonthDay limits occurrences to these days of the month; negative
	// values count back from the end, -1 being the last day.
	ByMonthDay []int
	// ByYearDay limits occurrences to these days of the year, 1-366 or
	// -1 to -366 counting back from the end.
	ByYearDay []int
	// ByWeekNo limits occurrences to these ISO 8601 week numbers, 1-53 or
	// negative counting back from the end of the year. It only applies to
	// FREQ=YEARLY.
	ByWeekNo []int
	// ByWeekday limits occurrences to these weekdays. Entries carrying an
	// ordinal (see Weekday.Nth) select the nth such weekday of the MONTHLY or
	// YEARLY interval; for finer frequencies the ordinal is ignored.
	ByWeekday []Weekday
	// ByHour limits or expands occurrences to these hours, 0-23.
	ByHour []int
	// ByMinute limits or expands occurrences to these minutes, 0-59.
	ByMinute []int
	// BySecond limits or expands occurrences to these seconds, 0-60.
	BySecond []int
}

// RRule is a compiled RFC 5545 recurrence rule. It is immutable once built by
// New, Parse or StrToRRule, and safe for concurrent use: every iteration
// method allocates its own state.
type RRule struct {
	orig Options // as supplied, for String
	loc  *time.Location

	freq     Freq
	dtstart  time.Time
	interval int
	wkst     int // ISO weekday, Monday = 0
	count    int
	until    time.Time
	hasUntil bool

	bysetpos    []int
	bymonth     []int
	byyearday   []int
	byweekno    []int
	bymonthday  []int    // positive days of the month
	bynmonthday []int    // negative days of the month
	byweekday   []int    // plain weekdays, ISO numbering
	bynweekday  [][2]int // {weekday, ordinal} pairs
	byhour      []int
	byminute    []int
	bysecond    []int

	timeset []timeOfDay // pre-computed times of day for FREQ coarser than HOURLY
}

// timeOfDay is a clock time within a day, without a date.
type timeOfDay struct{ h, m, s int }

// less orders times of day chronologically.
func (t timeOfDay) less(o timeOfDay) bool {
	if t.h != o.h {
		return t.h < o.h
	}
	if t.m != o.m {
		return t.m < o.m
	}
	return t.s < o.s
}

// New compiles opts into a recurrence rule, filling in the defaults RFC 5545
// derives from FREQ and DTSTART and validating every rule part. It returns an
// error wrapping ErrInvalidRule, ErrInvalidFreq or ErrEmptyRule if the options
// cannot describe a usable rule.
func New(opts Options) (*RRule, error) {
	r := &RRule{orig: opts}

	if opts.Freq < Yearly || opts.Freq > Secondly {
		return nil, fmt.Errorf("%w: %d", ErrInvalidFreq, int(opts.Freq))
	}
	r.freq = opts.Freq

	r.dtstart = opts.DTStart
	if r.dtstart.IsZero() {
		now := time.Now()
		r.dtstart = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, now.Location())
	} else {
		// Nanoseconds are not representable in RFC 5545 and would make
		// comparisons with generated occurrences fail.
		r.dtstart = r.dtstart.Truncate(time.Second)
	}
	r.loc = r.dtstart.Location()

	r.interval = opts.Interval
	if r.interval == 0 {
		r.interval = 1
	}
	if r.interval < 0 {
		return nil, fmt.Errorf("%w: INTERVAL must be positive, got %d", ErrInvalidRule, r.interval)
	}

	if opts.Wkst == 0 {
		r.wkst = pyWeekday(time.Monday)
	} else {
		if opts.Wkst < 0 || opts.Wkst > SundayStart {
			return nil, fmt.Errorf("%w: WKST out of range: %d", ErrInvalidRule, int(opts.Wkst))
		}
		r.wkst = pyWeekday(time.Weekday(int(opts.Wkst) % 7))
	}

	if opts.Count < 0 {
		return nil, fmt.Errorf("%w: COUNT must not be negative", ErrInvalidRule)
	}
	r.count = opts.Count
	if !opts.Until.IsZero() {
		r.until = opts.Until.Truncate(time.Second)
		r.hasUntil = true
	}

	for _, p := range opts.BySetPos {
		if p == 0 || p < -366 || p > 366 {
			return nil, fmt.Errorf("%w: BYSETPOS out of range: %d", ErrInvalidRule, p)
		}
	}
	r.bysetpos = sortedUnique(opts.BySetPos)

	// RFC 5545 §3.3.10: when no BYxxx rule part narrows the interval, the
	// corresponding component of DTSTART is implied.
	byMonth := opts.ByMonth
	byMonthDay := opts.ByMonthDay
	byWeekday := opts.ByWeekday
	if len(opts.ByWeekNo) == 0 && len(opts.ByYearDay) == 0 && len(opts.ByMonthDay) == 0 && len(opts.ByWeekday) == 0 {
		switch r.freq {
		case Yearly:
			if len(byMonth) == 0 {
				byMonth = []int{int(r.dtstart.Month())}
			}
			byMonthDay = []int{r.dtstart.Day()}
		case Monthly:
			byMonthDay = []int{r.dtstart.Day()}
		case Weekly:
			byWeekday = []Weekday{{Day: r.dtstart.Weekday()}}
		}
	}

	for _, m := range byMonth {
		if m < 1 || m > 12 {
			return nil, fmt.Errorf("%w: BYMONTH out of range: %d", ErrInvalidRule, m)
		}
	}
	r.bymonth = sortedUnique(byMonth)

	for _, d := range opts.ByYearDay {
		if d == 0 || d < -366 || d > 366 {
			return nil, fmt.Errorf("%w: BYYEARDAY out of range: %d", ErrInvalidRule, d)
		}
	}
	r.byyearday = sortedUnique(opts.ByYearDay)

	for _, w := range opts.ByWeekNo {
		if w == 0 || w < -53 || w > 53 {
			return nil, fmt.Errorf("%w: BYWEEKNO out of range: %d", ErrInvalidRule, w)
		}
	}
	r.byweekno = sortedUnique(opts.ByWeekNo)

	for _, d := range byMonthDay {
		if d == 0 || d < -31 || d > 31 {
			return nil, fmt.Errorf("%w: BYMONTHDAY out of range: %d", ErrInvalidRule, d)
		}
		if d > 0 {
			r.bymonthday = append(r.bymonthday, d)
		} else {
			r.bynmonthday = append(r.bynmonthday, d)
		}
	}
	r.bymonthday = sortedUnique(r.bymonthday)
	r.bynmonthday = sortedUnique(r.bynmonthday)

	for _, w := range byWeekday {
		if w.Day < time.Sunday || w.Day > time.Saturday {
			return nil, fmt.Errorf("%w: %v", ErrInvalidWeekday, w.Day)
		}
		if w.N == 0 || r.freq > Monthly {
			r.byweekday = append(r.byweekday, pyWeekday(w.Day))
		} else {
			r.bynweekday = append(r.bynweekday, [2]int{pyWeekday(w.Day), w.N})
		}
	}
	r.byweekday = sortedUnique(r.byweekday)
	sort.Slice(r.bynweekday, func(i, j int) bool {
		if r.bynweekday[i][0] != r.bynweekday[j][0] {
			return r.bynweekday[i][0] < r.bynweekday[j][0]
		}
		return r.bynweekday[i][1] < r.bynweekday[j][1]
	})

	var err error
	if r.byhour, err = r.constructTimePart(opts.ByHour, Hourly, r.dtstart.Hour(), 24, 0, 23, "BYHOUR"); err != nil {
		return nil, err
	}
	if r.byminute, err = r.constructTimePart(opts.ByMinute, Minutely, r.dtstart.Minute(), 60, 0, 59, "BYMINUTE"); err != nil {
		return nil, err
	}
	if r.bysecond, err = r.constructTimePart(opts.BySecond, Secondly, r.dtstart.Second(), 60, 0, 60, "BYSECOND"); err != nil {
		return nil, err
	}

	if r.freq < Hourly {
		for _, h := range r.byhour {
			for _, m := range r.byminute {
				for _, s := range r.bysecond {
					r.timeset = append(r.timeset, timeOfDay{h, m, s})
				}
			}
		}
		sort.Slice(r.timeset, func(i, j int) bool { return r.timeset[i].less(r.timeset[j]) })
	}
	return r, nil
}

// constructTimePart normalizes one of BYHOUR/BYMINUTE/BYSECOND. When the rule
// part is absent it defaults to the matching component of DTSTART for every
// frequency coarser than the part's own level, and to "every value" at or
// below that level. When the rule part is given at exactly its own level, the
// values unreachable from DTSTART by whole INTERVAL steps are dropped, per the
// same rule dateutil applies; if that leaves nothing, the rule is empty.
func (r *RRule) constructTimePart(given []int, level Freq, dflt, base, lo, hi int, name string) ([]int, error) {
	if len(given) == 0 {
		if r.freq < level {
			return []int{dflt}, nil
		}
		return nil, nil
	}
	for _, v := range given {
		if v < lo || v > hi {
			return nil, fmt.Errorf("%w: %s out of range: %d", ErrInvalidRule, name, v)
		}
	}
	if r.freq != level {
		return sortedUnique(given), nil
	}
	var out []int
	g := gcd(r.interval, base)
	for _, v := range given {
		if g == 1 || mod(v-dflt, g) == 0 {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s is unreachable with INTERVAL=%d", ErrEmptyRule, name, r.interval)
	}
	return sortedUnique(out), nil
}

// Options returns a copy of the options the rule was built from, with the
// defaults New applied for INTERVAL, DTSTART and WKST filled in.
func (r *RRule) Options() Options {
	o := r.orig
	o.DTStart = r.dtstart
	o.Interval = r.interval
	o.Wkst = goWeekday(r.wkst)
	if o.Wkst == time.Sunday {
		o.Wkst = SundayStart
	}
	return o
}

// DTStart returns the instant the recurrence is anchored to.
func (r *RRule) DTStart() time.Time { return r.dtstart }

// Freq returns the base frequency of the rule.
func (r *RRule) Freq() Freq { return r.freq }

// unbounded reports whether the rule has neither COUNT nor UNTIL.
func (r *RRule) unbounded() bool { return r.count == 0 && !r.hasUntil }

// All returns every occurrence of the rule.
//
// A rule with neither COUNT nor UNTIL is infinite; rather than never
// returning, All stops after MaxAllOccurrences occurrences. Prefer giving the
// rule a COUNT or an UNTIL, or use Between or Iterator, when the rule may be
// unbounded.
func (r *RRule) All() []time.Time {
	limit := -1
	if r.unbounded() {
		limit = MaxAllOccurrences
	}
	return collect(r.iterator(), limit)
}

// Between returns the occurrences that fall between after and before. The
// bounds are exclusive unless inc is true, in which case occurrences exactly
// equal to either bound are included.
func (r *RRule) Between(after, before time.Time, inc bool) []time.Time {
	return between(r.iterator(), after, before, inc)
}

// After returns the first occurrence strictly after t, or exactly at t when
// inc is true. It returns the zero time.Time if the rule has no such
// occurrence.
func (r *RRule) After(t time.Time, inc bool) time.Time {
	return afterFn(r.iterator(), t, inc)
}

// Before returns the last occurrence strictly before t, or exactly at t when
// inc is true. It returns the zero time.Time if the rule has no such
// occurrence.
func (r *RRule) Before(t time.Time, inc bool) time.Time {
	return beforeFn(r.iterator(), t, inc)
}

// Iterator returns a function that yields the occurrences of the rule in
// chronological order. The second result is false once the rule is exhausted;
// after that the function keeps returning false. The iterator holds its own
// state, so several may run at once.
func (r *RRule) Iterator() func() (time.Time, bool) {
	return r.iterator()
}

// collect drains next, stopping after limit values when limit is not negative.
func collect(next func() (time.Time, bool), limit int) []time.Time {
	out := []time.Time{}
	for limit < 0 || len(out) < limit {
		t, ok := next()
		if !ok {
			break
		}
		out = append(out, t)
	}
	return out
}

// between filters an occurrence stream to the given window.
func between(next func() (time.Time, bool), after, before time.Time, inc bool) []time.Time {
	out := []time.Time{}
	for {
		t, ok := next()
		if !ok {
			return out
		}
		if inc {
			if t.Before(after) {
				continue
			}
			if t.After(before) {
				return out
			}
		} else {
			if !t.After(after) {
				continue
			}
			if !t.Before(before) {
				return out
			}
		}
		out = append(out, t)
	}
}

// afterFn returns the first occurrence at or after t.
func afterFn(next func() (time.Time, bool), t time.Time, inc bool) time.Time {
	for {
		v, ok := next()
		if !ok {
			return time.Time{}
		}
		if v.After(t) || (inc && v.Equal(t)) {
			return v
		}
	}
}

// beforeFn returns the last occurrence at or before t.
func beforeFn(next func() (time.Time, bool), t time.Time, inc bool) time.Time {
	var last time.Time
	for {
		v, ok := next()
		if !ok {
			return last
		}
		if v.Before(t) || (inc && v.Equal(t)) {
			last = v
			continue
		}
		return last
	}
}
