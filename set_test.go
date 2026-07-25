package rrule

import (
	"strings"
	"testing"
	"time"
)

// implTuThRule is "every Tuesday and Thursday", the rule dateutil's own set
// tests are built on.
func implTuThRule(t *testing.T, count int) *RRule {
	t.Helper()
	return implNew(t, Options{Freq: Weekly, Count: count, ByWeekday: []Weekday{TU, TH}})
}

func TestSetMergesRules(t *testing.T) {
	s := NewSet()
	s.RRule(implNew(t, Options{Freq: Yearly, Count: 2, ByWeekday: []Weekday{TU}}))
	s.RRule(implNew(t, Options{Freq: Yearly, Count: 1, ByWeekday: []Weekday{TH}}))
	implEqual(t, "merged", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-04 09:00:00", "1997-09-09 09:00:00"))
}

func TestSetDeduplicates(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 4))
	s.RRule(implTuThRule(t, 4))
	s.RDate(time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC))
	implEqual(t, "deduplicated", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-04 09:00:00", "1997-09-09 09:00:00", "1997-09-11 09:00:00"))
}

func TestSetRDate(t *testing.T) {
	s := NewSet()
	s.RRule(implNew(t, Options{Freq: Yearly, Count: 1, ByWeekday: []Weekday{TU}}))
	s.RDate(time.Date(1997, 9, 9, 9, 0, 0, 0, time.UTC))
	s.RDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))
	implEqual(t, "rdate", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-04 09:00:00", "1997-09-09 09:00:00"))
}

func TestSetExDate(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 6))
	s.ExDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))
	s.ExDate(time.Date(1997, 9, 11, 9, 0, 0, 0, time.UTC))
	implEqual(t, "exdate", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-16 09:00:00", "1997-09-18 09:00:00"))
}

// TestSetExDateOutOfOrder checks that exclusions need not be added in order.
func TestSetExDateOutOfOrder(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 6))
	s.ExDate(time.Date(1997, 9, 11, 9, 0, 0, 0, time.UTC))
	s.ExDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))
	implEqual(t, "exdate reversed", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-16 09:00:00", "1997-09-18 09:00:00"))
}

func TestSetExRule(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 6))
	s.ExRule(implNew(t, Options{Freq: Weekly, Count: 3, ByWeekday: []Weekday{TH}}))
	implEqual(t, "exrule", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-16 09:00:00"))
}

// TestSetExRuleRemovesRDates checks that an exclusion applies to explicit
// dates as well as to rule output.
func TestSetExRuleRemovesRDates(t *testing.T) {
	s := NewSet()
	for _, d := range []int{2, 4, 9, 11, 16, 18} {
		s.RDate(time.Date(1997, 9, d, 9, 0, 0, 0, time.UTC))
	}
	s.ExRule(implNew(t, Options{Freq: Yearly, Count: 3, ByWeekday: []Weekday{TH}}))
	implEqual(t, "exrule over rdates", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-16 09:00:00"))
}

func TestSetWindowQueries(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 6))
	s.ExDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))

	from := time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC)
	to := time.Date(1997, 9, 16, 9, 0, 0, 0, time.UTC)
	implEqual(t, "set between", s.Between(from, to, true), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-11 09:00:00", "1997-09-16 09:00:00"))
	if got := s.After(from, false); !got.Equal(time.Date(1997, 9, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("set After = %v", got)
	}
	if got := s.Before(to, false); !got.Equal(time.Date(1997, 9, 11, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("set Before = %v", got)
	}
	if !s.DTStart().Equal(from) {
		t.Fatalf("set DTStart = %v", s.DTStart())
	}
}

func TestSetStringAndParse(t *testing.T) {
	s := NewSet()
	s.RRule(implTuThRule(t, 6))
	s.ExDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))
	s.RDate(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC))

	text := s.String()
	for _, want := range []string{
		"DTSTART:19970902T090000Z",
		"RRULE:FREQ=WEEKLY;COUNT=6;BYDAY=TU,TH",
		"RDATE:19970905T090000Z",
		"EXDATE:19970904T090000Z",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Set.String() = %q, missing %q", text, want)
		}
	}

	back, err := StrToSet(text)
	if err != nil {
		t.Fatalf("StrToSet(%q): %v", text, err)
	}
	implEqual(t, "set round trip", back.All(), s.All())
}

func TestEmptySet(t *testing.T) {
	s := NewSet()
	if got := s.All(); len(got) != 0 {
		t.Fatalf("empty set produced %v", got)
	}
	if !s.After(time.Now(), true).IsZero() || !s.Before(time.Now(), true).IsZero() {
		t.Fatal("empty set answered a window query")
	}
	if !s.DTStart().IsZero() {
		t.Fatal("empty set has a DTSTART")
	}
}
