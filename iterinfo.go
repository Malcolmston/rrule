package rrule

import "time"

// iterInfo holds the per-year and per-month lookup tables the recurrence
// engine filters against. Everything is indexed by day-of-year, zero based,
// and every table is seven days longer than the year so that a weekly
// interval may run past 31 December without special casing.
type iterInfo struct {
	r *RRule

	year      int
	lastYear  int
	lastMonth int

	yearLen     int
	nextYearLen int
	yearWeekday int // ISO weekday of 1 January, Monday = 0

	mmask     []int // month, 1-12, of each day of the year
	mdaymask  []int // day of the month, 1-31
	nmdaymask []int // day of the month counted from the end, -31..-1
	wdaymask  []int // ISO weekday, Monday = 0
	mrange    []int // 13 entries: first day-of-year index of each month

	wnomask   []int // 1 where BYWEEKNO matches
	nwdaymask []int // 1 where an ordinal BYDAY entry matches
}

// newIterInfo returns an iterInfo bound to r with no year loaded yet.
func newIterInfo(r *RRule) *iterInfo {
	return &iterInfo{r: r, lastYear: -1, lastMonth: -1}
}

// dayIndex returns the zero-based day-of-year index of a date in the
// currently loaded year.
func (ii *iterInfo) dayIndex(year, month, day int) int {
	return ii.mrange[month-1] + day - 1
}

// rebuild loads the tables for the given year and month, doing no work when
// they are already current.
func (ii *iterInfo) rebuild(year, month int) {
	r := ii.r
	if year != ii.lastYear {
		ii.year = year
		ii.yearLen = daysInYear(year)
		ii.nextYearLen = daysInYear(year + 1)
		ii.yearWeekday = pyWeekday(time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Weekday())

		n := ii.yearLen + 7
		ii.mmask = make([]int, n)
		ii.mdaymask = make([]int, n)
		ii.nmdaymask = make([]int, n)
		ii.mrange = make([]int, 13)

		idx := 0
		for m := 1; m <= 12; m++ {
			ii.mrange[m-1] = idx
			dim := daysInMonth(year, m)
			for d := 1; d <= dim; d++ {
				ii.mmask[idx] = m
				ii.mdaymask[idx] = d
				ii.nmdaymask[idx] = d - dim - 1
				idx++
			}
		}
		ii.mrange[12] = idx
		// The seven trailing days belong to January of the following year.
		for k := 0; k < 7; k++ {
			ii.mmask[idx+k] = 1
			ii.mdaymask[idx+k] = k + 1
			ii.nmdaymask[idx+k] = k - 31
		}

		// The weekday mask must be readable a little past the end of the
		// year: the BYWEEKNO builder and the weekly day set both look one
		// step ahead.
		ii.wdaymask = make([]int, ii.yearLen+14)
		for i := range ii.wdaymask {
			ii.wdaymask[i] = (ii.yearWeekday + i) % 7
		}

		ii.rebuildWeekNo()
	}

	if len(r.bynweekday) > 0 && (month != ii.lastMonth || year != ii.lastYear) {
		ii.rebuildNWeekday(month)
	}

	ii.lastYear = year
	ii.lastMonth = month
}

// rebuildWeekNo fills wnomask with the days covered by the BYWEEKNO rule
// part, using ISO 8601 week numbering relative to the rule's week start.
func (ii *iterInfo) rebuildWeekNo() {
	r := ii.r
	if len(r.byweekno) == 0 {
		ii.wnomask = nil
		return
	}
	ii.wnomask = make([]int, ii.yearLen+7)

	firstwkst := mod(7-ii.yearWeekday+r.wkst, 7)
	no1wkst := firstwkst
	var wyearlen int
	if no1wkst >= 4 {
		no1wkst = 0
		// The year keeps the days it inherited from the previous year.
		wyearlen = ii.yearLen + mod(ii.yearWeekday-r.wkst, 7)
	} else {
		// The days before the first week start belong to the previous year.
		wyearlen = ii.yearLen - no1wkst
	}
	div, rem := divmod(wyearlen, 7)
	numweeks := div + rem/4

	for _, n := range r.byweekno {
		if n < 0 {
			n += numweeks + 1
		}
		if n <= 0 || n > numweeks {
			continue
		}
		i := no1wkst
		if n > 1 {
			i = no1wkst + (n-1)*7
			if no1wkst != firstwkst {
				i -= 7 - firstwkst
			}
		}
		for j := 0; j < 7; j++ {
			ii.wnomask[i] = 1
			i++
			if ii.wdaymask[i] == r.wkst {
				break
			}
		}
	}

	if containsInt(r.byweekno, 1) {
		// Week 1 of the following year may start inside this one.
		i := no1wkst + numweeks*7
		if no1wkst != firstwkst {
			i -= 7 - firstwkst
		}
		if i < ii.yearLen {
			for j := 0; j < 7; j++ {
				ii.wnomask[i] = 1
				i++
				if ii.wdaymask[i] == r.wkst {
					break
				}
			}
		}
	}

	if no1wkst > 0 {
		// The last week of the previous year may reach into this one. When
		// no1wkst is zero it cannot: the year either starts on the week start
		// or week 1 took the leftover days.
		lnumweeks := -1
		if !containsInt(r.byweekno, -1) {
			lyearweekday := pyWeekday(time.Date(ii.year-1, time.January, 1, 0, 0, 0, 0, time.UTC).Weekday())
			lno1wkst := mod(7-lyearweekday+r.wkst, 7)
			lyearlen := daysInYear(ii.year - 1)
			if lno1wkst >= 4 {
				lnumweeks = 52 + mod(lyearlen+mod(lyearweekday-r.wkst, 7), 7)/4
			} else {
				lnumweeks = 52 + mod(ii.yearLen-no1wkst, 7)/4
			}
		}
		if containsInt(r.byweekno, lnumweeks) {
			for i := 0; i < no1wkst; i++ {
				ii.wnomask[i] = 1
			}
		}
	}
}

// rebuildNWeekday fills nwdaymask with the days matched by ordinal BYDAY
// entries such as "-1FR", which are only meaningful for FREQ=MONTHLY and
// FREQ=YEARLY.
func (ii *iterInfo) rebuildNWeekday(month int) {
	r := ii.r
	var ranges [][2]int
	switch r.freq {
	case Yearly:
		if len(r.bymonth) > 0 {
			for _, m := range r.bymonth {
				ranges = append(ranges, [2]int{ii.mrange[m-1], ii.mrange[m]})
			}
		} else {
			ranges = append(ranges, [2]int{0, ii.yearLen})
		}
	case Monthly:
		ranges = append(ranges, [2]int{ii.mrange[month-1], ii.mrange[month]})
	default:
		// Finer frequencies demote ordinal weekdays to plain ones, so there
		// is nothing to mask.
		return
	}

	ii.nwdaymask = make([]int, ii.yearLen)
	for _, rg := range ranges {
		first, last := rg[0], rg[1]-1
		for _, wd := range r.bynweekday {
			wday, n := wd[0], wd[1]
			var i int
			if n < 0 {
				i = last + (n+1)*7
				if i < 0 || i >= len(ii.wdaymask) {
					continue
				}
				i -= mod(ii.wdaymask[i]-wday, 7)
			} else {
				i = first + (n-1)*7
				if i < 0 || i >= len(ii.wdaymask) {
					continue
				}
				i += mod(7-ii.wdaymask[i]+wday, 7)
			}
			if first <= i && i <= last {
				ii.nwdaymask[i] = 1
			}
		}
	}
}
