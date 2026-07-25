package rrule

import (
	"sort"
	"time"
)

// iter is one live traversal of a rule. The engine follows the structure
// dateutil uses: for each frequency interval it builds the set of candidate
// days, removes the days the BYxxx rule parts reject, applies BYSETPOS to what
// is left, and emits the survivors in order before advancing to the next
// interval.
type iter struct {
	r  *RRule
	ii *iterInfo

	year, month, day     int
	hour, minute, second int
	weekday              int // ISO weekday of the current day, Monday = 0

	timeset []timeOfDay
	dset    []int // day-of-year candidate buffer, -1 meaning "no candidate"

	remaining int // occurrences left when the rule has a COUNT
	empty     int // consecutive intervals that produced nothing
	done      bool

	buf []time.Time
	pos int

	lastOut time.Time // the instant emitted last, for duplicate suppression
	hasLast bool
}

// iterator returns the raw occurrence generator for the rule.
func (r *RRule) iterator() func() (time.Time, bool) {
	it := r.newIter()
	return it.next
}

// newIter primes a traversal at DTSTART.
func (r *RRule) newIter() *iter {
	it := &iter{
		r:         r,
		ii:        newIterInfo(r),
		year:      r.dtstart.Year(),
		month:     int(r.dtstart.Month()),
		day:       r.dtstart.Day(),
		hour:      r.dtstart.Hour(),
		minute:    r.dtstart.Minute(),
		second:    r.dtstart.Second(),
		weekday:   pyWeekday(r.dtstart.Weekday()),
		remaining: r.count,
		dset:      make([]int, 366+7),
	}
	for i := range it.dset {
		it.dset[i] = -1
	}
	it.ii.rebuild(it.year, it.month)

	if r.freq < Hourly {
		it.timeset = r.timeset
	} else {
		// A rule whose FREQ is at or below the level of a BYxxx time part
		// starts inside the interval only if DTSTART already agrees with it.
		if (r.freq >= Hourly && len(r.byhour) > 0 && !containsInt(r.byhour, it.hour)) ||
			(r.freq >= Minutely && len(r.byminute) > 0 && !containsInt(r.byminute, it.minute)) ||
			(r.freq >= Secondly && len(r.bysecond) > 0 && !containsInt(r.bysecond, it.second)) {
			it.timeset = nil
		} else {
			it.timeset = it.getTimeset(it.hour, it.minute, it.second)
		}
	}
	return it
}

// next yields the following occurrence, or false once the rule is exhausted.
func (it *iter) next() (time.Time, bool) {
	for {
		if it.pos < len(it.buf) {
			t := it.buf[it.pos]
			it.pos++
			return t, true
		}
		if it.done {
			return time.Time{}, false
		}
		it.step()
	}
}

// stop marks the traversal finished.
func (it *iter) stop() { it.done = true }

// step evaluates one frequency interval, filling buf with whatever it yields,
// and advances the cursor to the next interval.
func (it *iter) step() {
	r := it.r
	ii := it.ii

	it.buf = it.buf[:0]
	it.pos = 0

	start, end := it.getDayset()
	filtered := it.filter(start, end)
	it.emit(start, end)
	it.clearDayset(start, end)

	if len(it.buf) > 0 {
		it.empty = 0
	} else {
		it.empty++
		if it.empty > maxEmptyIterations {
			// The rule is impossible, or so sparse that it is indistinguishable
			// from impossible. Give up rather than spin.
			it.stop()
			return
		}
	}
	if it.done {
		return
	}

	interval := r.interval
	fixday := false

	switch r.freq {
	case Yearly:
		it.year += interval
		if it.year > maxYear {
			it.stop()
			return
		}
		ii.rebuild(it.year, it.month)
	case Monthly:
		it.month += interval
		if it.month > 12 {
			div, m := divmod(it.month, 12)
			it.month = m
			it.year += div
			if it.month == 0 {
				it.month = 12
				it.year--
			}
			if it.year > maxYear {
				it.stop()
				return
			}
		}
		ii.rebuild(it.year, it.month)
	case Weekly:
		if r.wkst > it.weekday {
			it.day += -(it.weekday + 1 + (6 - r.wkst)) + interval*7
		} else {
			it.day += -(it.weekday - r.wkst) + interval*7
		}
		it.weekday = r.wkst
		fixday = true
	case Daily:
		it.day += interval
		fixday = true
	case Hourly:
		if filtered {
			// Every day in this interval was rejected; jump to the last hour
			// of the day so the next step lands on the following one.
			it.hour += ((23 - it.hour) / interval) * interval
		}
		var ndays int
		if len(r.byhour) > 0 {
			ndays, it.hour = it.modDistance(it.hour, r.byhour, 24)
		} else {
			ndays, it.hour = divmod(it.hour+interval, 24)
		}
		if ndays != 0 {
			it.day += ndays
			fixday = true
		}
		it.timeset = it.getTimeset(it.hour, it.minute, it.second)
	case Minutely:
		if filtered {
			it.minute += ((1439 - (it.hour*60 + it.minute)) / interval) * interval
		}
		valid := false
		repRate := 24 * 60
		for j := 0; j < repRate/gcd(interval, repRate); j++ {
			var nhours int
			if len(r.byminute) > 0 {
				nhours, it.minute = it.modDistance(it.minute, r.byminute, 60)
			} else {
				nhours, it.minute = divmod(it.minute+interval, 60)
			}
			var div int
			div, it.hour = divmod(it.hour+nhours, 24)
			if div != 0 {
				it.day += div
				fixday = true
				filtered = false
			}
			if len(r.byhour) == 0 || containsInt(r.byhour, it.hour) {
				valid = true
				break
			}
		}
		if !valid {
			// INTERVAL and BYHOUR can never agree.
			it.stop()
			return
		}
		it.timeset = it.getTimeset(it.hour, it.minute, it.second)
	case Secondly:
		if filtered {
			it.second += ((86399 - (it.hour*3600 + it.minute*60 + it.second)) / interval) * interval
		}
		valid := false
		repRate := 24 * 3600
		for j := 0; j < repRate/gcd(interval, repRate); j++ {
			var nminutes int
			if len(r.bysecond) > 0 {
				nminutes, it.second = it.modDistance(it.second, r.bysecond, 60)
			} else {
				nminutes, it.second = divmod(it.second+interval, 60)
			}
			var div int
			div, it.minute = divmod(it.minute+nminutes, 60)
			if div != 0 {
				it.hour += div
				div, it.hour = divmod(it.hour, 24)
				if div != 0 {
					it.day += div
					fixday = true
					filtered = false
				}
			}
			if (len(r.byhour) == 0 || containsInt(r.byhour, it.hour)) &&
				(len(r.byminute) == 0 || containsInt(r.byminute, it.minute)) &&
				(len(r.bysecond) == 0 || containsInt(r.bysecond, it.second)) {
				valid = true
				break
			}
		}
		if !valid {
			it.stop()
			return
		}
		it.timeset = it.getTimeset(it.hour, it.minute, it.second)
	}

	if fixday && it.day > 28 {
		dim := daysInMonth(it.year, it.month)
		if it.day > dim {
			for it.day > dim {
				it.day -= dim
				it.month++
				if it.month == 13 {
					it.month = 1
					it.year++
					if it.year > maxYear {
						it.stop()
						return
					}
				}
				dim = daysInMonth(it.year, it.month)
			}
			ii.rebuild(it.year, it.month)
		}
	}
}

// getDayset marks the candidate days of the current interval in it.dset and
// returns the half-open range of day-of-year indices they occupy.
func (it *iter) getDayset() (int, int) {
	ii := it.ii
	switch it.r.freq {
	case Yearly:
		for i := 0; i < ii.yearLen; i++ {
			it.dset[i] = i
		}
		return 0, ii.yearLen
	case Monthly:
		start, end := ii.mrange[it.month-1], ii.mrange[it.month]
		for i := start; i < end; i++ {
			it.dset[i] = i
		}
		return start, end
	case Weekly:
		// A weekly interval may cross into the next year, which is why the
		// masks run seven days long.
		i := ii.dayIndex(it.year, it.month, it.day)
		start := i
		for j := 0; j < 7; j++ {
			it.dset[i] = i
			i++
			if ii.wdaymask[i] == it.r.wkst {
				break
			}
		}
		return start, i
	default:
		i := ii.dayIndex(it.year, it.month, it.day)
		it.dset[i] = i
		return i, i + 1
	}
}

// clearDayset resets the buffer so the next interval starts from a clean one.
func (it *iter) clearDayset(start, end int) {
	for i := start; i < end; i++ {
		it.dset[i] = -1
	}
}

// filter removes from the candidate set every day rejected by a BYxxx rule
// part. It reports whether anything was removed, which the sub-daily
// frequencies use to skip ahead to the next day.
func (it *iter) filter(start, end int) bool {
	r := it.r
	ii := it.ii
	filtered := false
	for j := start; j < end; j++ {
		i := it.dset[j]
		if i < 0 {
			continue
		}
		drop := false
		switch {
		case len(r.bymonth) > 0 && !containsInt(r.bymonth, ii.mmask[i]):
			drop = true
		case len(r.byweekno) > 0 && ii.wnomask[i] == 0:
			drop = true
		case len(r.byweekday) > 0 && !containsInt(r.byweekday, ii.wdaymask[i]):
			drop = true
		case len(ii.nwdaymask) > 0 && i < len(ii.nwdaymask) && ii.nwdaymask[i] == 0:
			drop = true
		case (len(r.bymonthday) > 0 || len(r.bynmonthday) > 0) &&
			!containsInt(r.bymonthday, ii.mdaymask[i]) &&
			!containsInt(r.bynmonthday, ii.nmdaymask[i]):
			drop = true
		case len(r.byyearday) > 0 && !it.matchYearDay(i):
			drop = true
		}
		if drop {
			it.dset[j] = -1
			filtered = true
		}
	}
	return filtered
}

// matchYearDay reports whether day-of-year index i satisfies BYYEARDAY,
// accounting for indices that have run past the end of the year and for
// negative values counting back from 31 December.
func (it *iter) matchYearDay(i int) bool {
	r := it.r
	ii := it.ii
	if i < ii.yearLen {
		return containsInt(r.byyearday, i+1) || containsInt(r.byyearday, i-ii.yearLen)
	}
	return containsInt(r.byyearday, i+1-ii.yearLen) || containsInt(r.byyearday, i-ii.yearLen-ii.nextYearLen)
}

// emit turns the surviving candidate days into occurrences, applying BYSETPOS,
// UNTIL, COUNT and the DTSTART lower bound.
func (it *iter) emit(start, end int) {
	r := it.r

	if len(r.bysetpos) > 0 && len(it.timeset) > 0 {
		var days []int
		for j := start; j < end; j++ {
			if it.dset[j] >= 0 {
				days = append(days, it.dset[j])
			}
		}
		var poslist []time.Time
		for _, pos := range r.bysetpos {
			var daypos, timepos int
			if pos < 0 {
				daypos, timepos = divmod(pos, len(it.timeset))
			} else {
				daypos, timepos = divmod(pos-1, len(it.timeset))
			}
			idx := daypos
			if idx < 0 {
				idx += len(days)
			}
			if idx < 0 || idx >= len(days) {
				continue
			}
			res := it.combine(days[idx], it.timeset[timepos])
			dup := false
			for _, p := range poslist {
				if p.Equal(res) {
					dup = true
					break
				}
			}
			if !dup {
				poslist = append(poslist, res)
			}
		}
		sort.Slice(poslist, func(i, j int) bool { return poslist[i].Before(poslist[j]) })
		for _, res := range poslist {
			if !it.yield(res) {
				return
			}
		}
		return
	}

	for j := start; j < end; j++ {
		i := it.dset[j]
		if i < 0 {
			continue
		}
		for _, t := range it.timeset {
			if !it.yield(it.combine(i, t)) {
				return
			}
		}
	}
}

// yield appends res to the output of the current interval unless it is before
// DTSTART, and reports whether iteration should continue.
func (it *iter) yield(res time.Time) bool {
	r := it.r
	if r.hasUntil && res.After(r.until) {
		it.stop()
		return false
	}
	if res.Before(r.dtstart) {
		return true
	}
	if it.hasLast && res.Equal(it.lastOut) {
		// Two wall clocks can name the same instant when a daylight-saving
		// transition removes an hour: an HOURLY rule crossing the gap asks for
		// both 02:00 and 03:00, and 02:00 resolves to the same instant as
		// 03:00. Emit it once.
		return true
	}
	if r.count > 0 {
		if it.remaining == 0 {
			it.stop()
			return false
		}
		it.remaining--
	}
	it.buf = append(it.buf, res)
	it.lastOut, it.hasLast = res, true
	return true
}

// combine builds the occurrence for day-of-year index i at the given time of
// day, in the rule's location. Day arithmetic is done on wall-clock fields, so
// a daily rule keeps its local clock time across a daylight-saving transition;
// time.Date resolves a wall clock that a transition skipped by moving it
// forward, and an ambiguous one by taking the earlier offset.
func (it *iter) combine(i int, t timeOfDay) time.Time {
	return time.Date(it.ii.year, time.January, 1+i, t.h, t.m, t.s, 0, it.r.loc)
}

// getTimeset returns the times of day generated within one interval of an
// HOURLY, MINUTELY or SECONDLY rule.
func (it *iter) getTimeset(hour, minute, second int) []timeOfDay {
	r := it.r
	var out []timeOfDay
	switch r.freq {
	case Hourly:
		for _, m := range r.byminute {
			for _, s := range r.bysecond {
				out = append(out, timeOfDay{hour, m, s})
			}
		}
	case Minutely:
		for _, s := range r.bysecond {
			out = append(out, timeOfDay{hour, minute, s})
		}
	default:
		out = append(out, timeOfDay{hour, minute, second})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

// modDistance advances value by whole INTERVAL steps until it lands on a member
// of byxxx, returning how many times it wrapped past base and the new value.
// It is what lets FREQ=MINUTELY;INTERVAL=7;BYMINUTE=... skip forward without
// visiting every step one at a time.
func (it *iter) modDistance(value int, byxxx []int, base int) (int, int) {
	accumulator := 0
	for i := 0; i <= base; i++ {
		var div int
		div, value = divmod(value+it.r.interval, base)
		accumulator += div
		if containsInt(byxxx, value) {
			return accumulator, value
		}
	}
	return accumulator, value
}
