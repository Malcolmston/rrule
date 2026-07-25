package rrule

import (
	"sort"
	"strings"
	"time"
)

// Set is a recurrence set: any number of RRULE and RDATE inclusions minus any
// number of EXRULE and EXDATE exclusions, as RFC 5545 composes them inside a
// VEVENT.
//
// Occurrences from every inclusion are merged in chronological order and
// de-duplicated, then any instant produced by an exclusion is removed. The
// zero Set is usable and empty.
type Set struct {
	rrules  []*RRule
	exrules []*RRule
	rdates  []time.Time
	exdates []time.Time
}

// NewSet returns an empty recurrence set.
func NewSet() *Set { return &Set{} }

// RRule adds an inclusion rule to the set.
func (s *Set) RRule(r *RRule) {
	if r != nil {
		s.rrules = append(s.rrules, r)
	}
}

// ExRule adds an exclusion rule: every instant it generates is removed from
// the set, whether it came from an RRULE or an RDATE.
func (s *Set) ExRule(r *RRule) {
	if r != nil {
		s.exrules = append(s.exrules, r)
	}
}

// RDate adds a single explicit occurrence to the set.
func (s *Set) RDate(t time.Time) {
	s.rdates = append(s.rdates, t.Truncate(time.Second))
}

// ExDate removes a single instant from the set.
func (s *Set) ExDate(t time.Time) {
	s.exdates = append(s.exdates, t.Truncate(time.Second))
}

// RRules returns the inclusion rules of the set.
func (s *Set) RRules() []*RRule { return append([]*RRule(nil), s.rrules...) }

// ExRules returns the exclusion rules of the set.
func (s *Set) ExRules() []*RRule { return append([]*RRule(nil), s.exrules...) }

// RDates returns the explicit occurrences of the set, sorted.
func (s *Set) RDates() []time.Time { return sortedTimes(s.rdates) }

// ExDates returns the explicitly excluded instants of the set, sorted.
func (s *Set) ExDates() []time.Time { return sortedTimes(s.exdates) }

// DTStart returns the anchor of the first rule in the set, or the earliest
// RDATE when the set holds no rule. It is the zero time for an empty set.
func (s *Set) DTStart() time.Time {
	if len(s.rrules) > 0 {
		return s.rrules[0].DTStart()
	}
	if d := sortedTimes(s.rdates); len(d) > 0 {
		return d[0]
	}
	return time.Time{}
}

// All returns every occurrence of the set.
//
// As with RRule.All, a set containing an unbounded rule would be infinite, so
// All stops after MaxAllOccurrences occurrences.
func (s *Set) All() []time.Time {
	limit := -1
	for _, r := range s.rrules {
		if r.unbounded() {
			limit = MaxAllOccurrences
			break
		}
	}
	return collect(s.Iterator(), limit)
}

// Between returns the occurrences of the set that fall between after and
// before, exclusive unless inc is true.
func (s *Set) Between(after, before time.Time, inc bool) []time.Time {
	return between(s.Iterator(), after, before, inc)
}

// After returns the first occurrence of the set strictly after t, or exactly
// at t when inc is true, and the zero time.Time if there is none.
func (s *Set) After(t time.Time, inc bool) time.Time {
	return afterFn(s.Iterator(), t, inc)
}

// Before returns the last occurrence of the set strictly before t, or exactly
// at t when inc is true, and the zero time.Time if there is none.
func (s *Set) Before(t time.Time, inc bool) time.Time {
	return beforeFn(s.Iterator(), t, inc)
}

// Iterator returns a function yielding the occurrences of the set in
// chronological order, de-duplicated and with the exclusions removed.
func (s *Set) Iterator() func() (time.Time, bool) {
	inc := newMerge(s.rrules, s.rdates)
	exc := newMerge(s.exrules, s.exdates)

	var last time.Time
	var haveLast bool
	var exHead time.Time
	exValid := false
	exStarted := false

	return func() (time.Time, bool) {
		for {
			t, ok := inc.next()
			if !ok {
				return time.Time{}, false
			}
			if haveLast && t.Equal(last) {
				continue
			}
			last, haveLast = t, true

			if !exStarted {
				exHead, exValid = exc.next()
				exStarted = true
			}
			for exValid && exHead.Before(t) {
				exHead, exValid = exc.next()
			}
			if exValid && exHead.Equal(t) {
				continue
			}
			return t, true
		}
	}
}

// merge is a k-way chronological merge over rule iterators and a sorted list
// of explicit dates.
type merge struct {
	sources []func() (time.Time, bool)
	heads   []time.Time
	valid   []bool
	primed  bool
}

// newMerge builds a merge over the given rules plus the given explicit dates.
func newMerge(rules []*RRule, dates []time.Time) *merge {
	m := &merge{}
	for _, r := range rules {
		m.sources = append(m.sources, r.iterator())
	}
	if len(dates) > 0 {
		m.sources = append(m.sources, sliceIterator(sortedTimes(dates)))
	}
	m.heads = make([]time.Time, len(m.sources))
	m.valid = make([]bool, len(m.sources))
	return m
}

// next returns the earliest instant across every source, consuming every
// source that holds it.
func (m *merge) next() (time.Time, bool) {
	if !m.primed {
		for i, src := range m.sources {
			m.heads[i], m.valid[i] = src()
		}
		m.primed = true
	}
	best := -1
	for i := range m.sources {
		if !m.valid[i] {
			continue
		}
		if best < 0 || m.heads[i].Before(m.heads[best]) {
			best = i
		}
	}
	if best < 0 {
		return time.Time{}, false
	}
	res := m.heads[best]
	for i := range m.sources {
		if m.valid[i] && m.heads[i].Equal(res) {
			m.heads[i], m.valid[i] = m.sources[i]()
		}
	}
	return res, true
}

// sliceIterator yields the elements of a sorted slice.
func sliceIterator(ts []time.Time) func() (time.Time, bool) {
	i := 0
	return func() (time.Time, bool) {
		if i >= len(ts) {
			return time.Time{}, false
		}
		t := ts[i]
		i++
		return t, true
	}
}

// sortedTimes returns a chronologically sorted copy of ts.
func sortedTimes(ts []time.Time) []time.Time {
	out := append([]time.Time(nil), ts...)
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// String returns the set as RFC 5545 content lines: the DTSTART of the first
// rule, then one line per RRULE, EXRULE, RDATE and EXDATE. Explicit dates are
// gathered into a single comma-separated line each, as RFC 5545 permits.
func (s *Set) String() string {
	var lines []string
	if len(s.rrules) > 0 {
		lines = append(lines, s.rrules[0].dtstartLine())
	}
	for _, r := range s.rrules {
		lines = append(lines, "RRULE:"+r.RuleString())
	}
	for _, r := range s.exrules {
		lines = append(lines, "EXRULE:"+r.RuleString())
	}
	if d := s.RDates(); len(d) > 0 {
		lines = append(lines, "RDATE:"+joinTimes(d))
	}
	if d := s.ExDates(); len(d) > 0 {
		lines = append(lines, "EXDATE:"+joinTimes(d))
	}
	return strings.Join(lines, "\n")
}

// joinTimes renders a comma-separated RFC 5545 date-time list.
func joinTimes(ts []time.Time) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = formatICalTime(t)
	}
	return strings.Join(parts, ",")
}
