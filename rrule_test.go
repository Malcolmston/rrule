package rrule

import (
	"errors"
	"testing"
	"time"
)

// implStart is the anchor dateutil's own test suite uses, so the expectations
// below can be compared with it directly.
var implStart = time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC)

// implTimes builds a slice of UTC instants from "2006-01-02 15:04:05" strings.
func implTimes(t *testing.T, ss ...string) []time.Time {
	t.Helper()
	out := make([]time.Time, len(ss))
	for i, s := range ss {
		v, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC)
		if err != nil {
			t.Fatalf("bad test time %q: %v", s, err)
		}
		out[i] = v
	}
	return out
}

// implEqual fails the test unless got and want hold the same instants.
func implEqual(t *testing.T, name string, got, want []time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d occurrences %v, want %d %v", name, len(got), got, len(want), want)
	}
	for i := range got {
		if !got[i].Equal(want[i]) {
			t.Fatalf("%s: occurrence %d = %v, want %v", name, i, got[i], want[i])
		}
	}
}

// implNew builds a rule or fails the test.
func implNew(t *testing.T, o Options) *RRule {
	t.Helper()
	if o.DTStart.IsZero() {
		o.DTStart = implStart
	}
	r, err := New(o)
	if err != nil {
		t.Fatalf("New(%+v): %v", o, err)
	}
	return r
}

func TestFrequencies(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"yearly", Options{Freq: Yearly, Count: 3},
			[]string{"1997-09-02 09:00:00", "1998-09-02 09:00:00", "1999-09-02 09:00:00"}},
		{"monthly", Options{Freq: Monthly, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-10-02 09:00:00", "1997-11-02 09:00:00"}},
		{"weekly", Options{Freq: Weekly, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-09-09 09:00:00", "1997-09-16 09:00:00"}},
		{"daily", Options{Freq: Daily, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-09-03 09:00:00", "1997-09-04 09:00:00"}},
		{"hourly", Options{Freq: Hourly, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-09-02 10:00:00", "1997-09-02 11:00:00"}},
		{"minutely", Options{Freq: Minutely, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-09-02 09:01:00", "1997-09-02 09:02:00"}},
		{"secondly", Options{Freq: Secondly, Count: 3},
			[]string{"1997-09-02 09:00:00", "1997-09-02 09:00:01", "1997-09-02 09:00:02"}},
		{"interval", Options{Freq: Daily, Count: 3, Interval: 10},
			[]string{"1997-09-02 09:00:00", "1997-09-12 09:00:00", "1997-09-22 09:00:00"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			implEqual(t, c.name, implNew(t, c.opts).All(), implTimes(t, c.want...))
		})
	}
}

func TestByRuleParts(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"bymonth", Options{Freq: Yearly, Count: 3, ByMonth: []int{1, 3}},
			[]string{"1998-01-02 09:00:00", "1998-03-02 09:00:00", "1999-01-02 09:00:00"}},
		{"bymonthday negative", Options{Freq: Monthly, Count: 4, ByMonthDay: []int{-1}},
			[]string{"1997-09-30 09:00:00", "1997-10-31 09:00:00", "1997-11-30 09:00:00", "1997-12-31 09:00:00"}},
		{"byyearday", Options{Freq: Yearly, Count: 4, ByYearDay: []int{1, 100, -1}},
			[]string{"1997-12-31 09:00:00", "1998-01-01 09:00:00", "1998-04-10 09:00:00", "1998-12-31 09:00:00"}},
		{"byweekno", Options{Freq: Yearly, Count: 4, ByWeekNo: []int{20}},
			[]string{"1998-05-11 09:00:00", "1998-05-12 09:00:00", "1998-05-13 09:00:00", "1998-05-14 09:00:00"}},
		{"byweekday", Options{Freq: Weekly, Count: 4, ByWeekday: []Weekday{TU, TH}},
			[]string{"1997-09-02 09:00:00", "1997-09-04 09:00:00", "1997-09-09 09:00:00", "1997-09-11 09:00:00"}},
		{"byweekday ordinal", Options{Freq: Monthly, Count: 4, ByWeekday: []Weekday{FR.Nth(-1)}},
			[]string{"1997-09-26 09:00:00", "1997-10-31 09:00:00", "1997-11-28 09:00:00", "1997-12-26 09:00:00"}},
		{"byhour", Options{Freq: Daily, Count: 4, ByHour: []int{6, 18}},
			[]string{"1997-09-02 18:00:00", "1997-09-03 06:00:00", "1997-09-03 18:00:00", "1997-09-04 06:00:00"}},
		{"byminute", Options{Freq: Hourly, Count: 4, ByMinute: []int{15, 45}},
			[]string{"1997-09-02 09:15:00", "1997-09-02 09:45:00", "1997-09-02 10:15:00", "1997-09-02 10:45:00"}},
		{"bysecond", Options{Freq: Minutely, Count: 4, BySecond: []int{10, 50}},
			[]string{"1997-09-02 09:00:10", "1997-09-02 09:00:50", "1997-09-02 09:01:10", "1997-09-02 09:01:50"}},
		{"bysetpos last weekday", Options{Freq: Monthly, Count: 4,
			ByWeekday: []Weekday{MO, TU, WE, TH, FR}, BySetPos: []int{-1}},
			[]string{"1997-09-30 09:00:00", "1997-10-31 09:00:00", "1997-11-28 09:00:00", "1997-12-31 09:00:00"}},
		{"leap day only", Options{Freq: Yearly, Count: 3, ByMonth: []int{2}, ByMonthDay: []int{29},
			DTStart: time.Date(1997, 1, 1, 0, 0, 0, 0, time.UTC)},
			[]string{"2000-02-29 00:00:00", "2004-02-29 00:00:00", "2008-02-29 00:00:00"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			implEqual(t, c.name, implNew(t, c.opts).All(), implTimes(t, c.want...))
		})
	}
}

// TestWkstChangesResults pins the two ways an interval of two weeks can be cut,
// which is the clearest demonstration that WKST is threaded through the engine.
func TestWkstChangesResults(t *testing.T) {
	mo := implNew(t, Options{Freq: Weekly, Count: 4, Interval: 2,
		ByWeekday: []Weekday{TU, SU}, Wkst: time.Monday})
	implEqual(t, "wkst=MO", mo.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-07 09:00:00", "1997-09-16 09:00:00", "1997-09-21 09:00:00"))

	su := implNew(t, Options{Freq: Weekly, Count: 4, Interval: 2,
		ByWeekday: []Weekday{TU, SU}, Wkst: SundayStart})
	implEqual(t, "wkst=SU", su.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-14 09:00:00", "1997-09-16 09:00:00", "1997-09-28 09:00:00"))
}

// TestWkstAffectsWeekNo checks that ISO week 1 moves with the week start.
func TestWkstAffectsWeekNo(t *testing.T) {
	su := implNew(t, Options{Freq: Yearly, Count: 4, ByWeekNo: []int{1},
		ByWeekday: []Weekday{MO}, Wkst: SundayStart})
	implEqual(t, "byweekno wkst=SU", su.All(), implTimes(t,
		"1998-01-05 09:00:00", "1999-01-04 09:00:00", "2000-01-03 09:00:00", "2001-01-01 09:00:00"))
}

// TestWkstZeroValueIsMonday documents the SundayStart convention.
func TestWkstZeroValueIsMonday(t *testing.T) {
	implicit := implNew(t, Options{Freq: Weekly, Count: 4, Interval: 2, ByWeekday: []Weekday{TU, SU}})
	explicit := implNew(t, Options{Freq: Weekly, Count: 4, Interval: 2,
		ByWeekday: []Weekday{TU, SU}, Wkst: time.Monday})
	implEqual(t, "zero Wkst", implicit.All(), explicit.All())
}

func TestUntil(t *testing.T) {
	r := implNew(t, Options{Freq: Daily, Until: time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC)})
	implEqual(t, "until", r.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-03 09:00:00", "1997-09-04 09:00:00", "1997-09-05 09:00:00"))

	// UNTIL is inclusive only when an occurrence falls exactly on it.
	r2 := implNew(t, Options{Freq: Daily, Until: time.Date(1997, 9, 5, 8, 59, 59, 0, time.UTC)})
	if got := len(r2.All()); got != 3 {
		t.Fatalf("until just before an occurrence: got %d, want 3", got)
	}
}

func TestBetweenAfterBefore(t *testing.T) {
	r := implNew(t, Options{Freq: Daily, Count: 10})
	from := time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC)
	to := time.Date(1997, 9, 7, 9, 0, 0, 0, time.UTC)

	implEqual(t, "between exclusive", r.Between(from, to, false), implTimes(t,
		"1997-09-05 09:00:00", "1997-09-06 09:00:00"))
	implEqual(t, "between inclusive", r.Between(from, to, true), implTimes(t,
		"1997-09-04 09:00:00", "1997-09-05 09:00:00", "1997-09-06 09:00:00", "1997-09-07 09:00:00"))

	if got := r.After(from, false); !got.Equal(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("After exclusive = %v", got)
	}
	if got := r.After(from, true); !got.Equal(from) {
		t.Fatalf("After inclusive = %v", got)
	}
	if got := r.Before(from, false); !got.Equal(time.Date(1997, 9, 3, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("Before exclusive = %v", got)
	}
	if got := r.Before(from, true); !got.Equal(from) {
		t.Fatalf("Before inclusive = %v", got)
	}
	if got := r.After(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), true); !got.IsZero() {
		t.Fatalf("After past the end = %v, want the zero time", got)
	}
}

// TestIteratorsAreIndependent checks that two live traversals do not share
// state.
func TestIteratorsAreIndependent(t *testing.T) {
	r := implNew(t, Options{Freq: Daily, Count: 5})
	a, b := r.Iterator(), r.Iterator()
	first, _ := a()
	a()
	second, _ := b()
	if !first.Equal(second) {
		t.Fatalf("second iterator started at %v, want %v", second, first)
	}
	for i := 0; i < 5; i++ {
		a()
	}
	if _, ok := a(); ok {
		t.Fatal("iterator kept yielding past COUNT")
	}
	if _, ok := a(); ok {
		t.Fatal("exhausted iterator restarted")
	}
}

// TestImpossibleRuleTerminates is the termination guard: 31 February never
// happens, and the engine must give up rather than spin.
func TestImpossibleRuleTerminates(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		r, err := New(Options{Freq: Monthly, DTStart: implStart,
			ByMonth: []int{2}, ByMonthDay: []int{31}})
		if err != nil {
			t.Error(err)
			done <- -1
			return
		}
		done <- len(r.All())
	}()
	select {
	case n := <-done:
		if n != 0 {
			t.Fatalf("impossible rule produced %d occurrences", n)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("impossible rule did not terminate")
	}
}

// TestSparseRuleStillWorks guards the termination bound from being so tight
// that a legitimately sparse rule is cut short.
func TestSparseRuleStillWorks(t *testing.T) {
	r := implNew(t, Options{Freq: Daily, Count: 2, ByMonth: []int{2}, ByMonthDay: []int{29},
		DTStart: time.Date(1997, 1, 1, 0, 0, 0, 0, time.UTC)})
	implEqual(t, "leap days daily", r.All(), implTimes(t, "2000-02-29 00:00:00", "2004-02-29 00:00:00"))
}

// TestUnboundedAllIsCapped documents that All never runs forever.
func TestUnboundedAllIsCapped(t *testing.T) {
	r := implNew(t, Options{Freq: Daily})
	if got := len(r.All()); got != MaxAllOccurrences {
		t.Fatalf("unbounded All returned %d occurrences, want %d", got, MaxAllOccurrences)
	}
}

func TestDaylightSavingKeepsWallClock(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tz database")
	}
	r := implNew(t, Options{Freq: Daily, Count: 4,
		DTStart: time.Date(2023, 3, 10, 9, 0, 0, 0, ny)})
	got := r.All()
	for i, occ := range got {
		if occ.Hour() != 9 || occ.Minute() != 0 {
			t.Fatalf("occurrence %d = %v, want 09:00 local", i, occ)
		}
	}
	// The transition happened on the 12th, so the last two occurrences are an
	// hour closer together in absolute time than the first two.
	if d := got[1].Sub(got[0]); d != 24*time.Hour {
		t.Fatalf("before the transition the gap was %v", d)
	}
	if d := got[2].Sub(got[1]); d != 23*time.Hour {
		t.Fatalf("across the transition the gap was %v, want 23h", d)
	}
}

// TestDaylightSavingNoDuplicateInstants covers the spring-forward gap, where
// two wall clocks name one instant, and the autumn overlap, where one wall
// clock names two.
func TestDaylightSavingNoDuplicateInstants(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tz database")
	}
	for _, c := range []struct {
		name  string
		start time.Time
	}{
		{"spring forward", time.Date(2023, 3, 12, 0, 0, 0, 0, ny)},
		{"fall back", time.Date(2023, 11, 5, 0, 0, 0, 0, ny)},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := implNew(t, Options{Freq: Hourly, Count: 6, DTStart: c.start})
			got := r.All()
			seen := map[int64]bool{}
			for i, occ := range got {
				if seen[occ.Unix()] {
					t.Fatalf("occurrence %d (%v) repeats an instant", i, occ)
				}
				seen[occ.Unix()] = true
				if i > 0 && !occ.After(got[i-1]) {
					t.Fatalf("occurrence %d (%v) is not after %v", i, occ, got[i-1])
				}
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want error
	}{
		{"bad freq", Options{Freq: Freq(99)}, ErrInvalidFreq},
		{"negative interval", Options{Freq: Daily, Interval: -1}, ErrInvalidRule},
		{"bad month", Options{Freq: Yearly, ByMonth: []int{13}}, ErrInvalidRule},
		{"zero monthday", Options{Freq: Monthly, ByMonthDay: []int{0}}, ErrInvalidRule},
		{"zero setpos", Options{Freq: Monthly, BySetPos: []int{0}}, ErrInvalidRule},
		{"bad hour", Options{Freq: Daily, ByHour: []int{24}}, ErrInvalidRule},
		{"unreachable byhour", Options{Freq: Hourly, Interval: 2, ByHour: []int{10},
			DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC)}, ErrEmptyRule},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := c.opts
			if opts.DTStart.IsZero() {
				opts.DTStart = implStart
			}
			_, err := New(opts)
			if !errors.Is(err, c.want) {
				t.Fatalf("New: err = %v, want one wrapping %v", err, c.want)
			}
		})
	}
}

// TestDTStartDefaultsFromFreq covers the RFC 5545 rule that an absent BYxxx
// part is taken from DTSTART.
func TestDTStartDefaultsFromFreq(t *testing.T) {
	yearly := implNew(t, Options{Freq: Yearly, Count: 2})
	if got := yearly.All()[1]; got.Month() != time.September || got.Day() != 2 {
		t.Fatalf("yearly default = %v, want September 2", got)
	}
	weekly := implNew(t, Options{Freq: Weekly, Count: 2})
	if got := weekly.All()[1]; got.Weekday() != time.Tuesday {
		t.Fatalf("weekly default = %v, want a Tuesday", got)
	}
	// The time of day of DTSTART supplies BYHOUR/BYMINUTE/BYSECOND.
	sec := implNew(t, Options{Freq: Daily, Count: 1,
		DTStart: time.Date(1997, 9, 2, 9, 30, 15, 0, time.UTC)})
	if got := sec.All()[0]; got.Minute() != 30 || got.Second() != 15 {
		t.Fatalf("time of day default = %v", got)
	}
}

func TestOptionsAccessors(t *testing.T) {
	r := implNew(t, Options{Freq: Monthly, Count: 3, ByWeekday: []Weekday{FR.Nth(-1)}})
	if r.Freq() != Monthly {
		t.Fatalf("Freq() = %v", r.Freq())
	}
	if !r.DTStart().Equal(implStart) {
		t.Fatalf("DTStart() = %v", r.DTStart())
	}
	o := r.Options()
	if o.Interval != 1 || o.Wkst != time.Monday {
		t.Fatalf("Options() defaults not filled in: %+v", o)
	}
}

func TestFreqAndWeekdayStrings(t *testing.T) {
	if Weekly.String() != "WEEKLY" || Secondly.String() != "SECONDLY" {
		t.Fatal("Freq.String is wrong")
	}
	if FR.Nth(-1).String() != "-1FR" || MO.String() != "MO" || TU.Nth(2).String() != "2TU" {
		t.Fatal("Weekday.String is wrong")
	}
}

// TestConcurrentIteration backs the claim that a compiled rule is safe to use
// from several goroutines; run it under -race.
func TestConcurrentIteration(t *testing.T) {
	r := implNew(t, Options{Freq: Monthly, Count: 24, ByWeekday: []Weekday{FR.Nth(-1)}})
	want := r.All()
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func() {
			got := r.All()
			if len(got) != len(want) {
				errs <- "length mismatch"
				return
			}
			for j := range got {
				if !got[j].Equal(want[j]) {
					errs <- "value mismatch"
					return
				}
			}
			errs <- ""
		}()
	}
	for i := 0; i < 8; i++ {
		if msg := <-errs; msg != "" {
			t.Fatal(msg)
		}
	}
}
