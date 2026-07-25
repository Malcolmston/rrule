package rrule

import (
	"errors"
	"testing"
	"time"
)

func TestParseBareAndPrefixed(t *testing.T) {
	for _, s := range []string{
		"FREQ=DAILY;COUNT=3",
		"RRULE:FREQ=DAILY;COUNT=3",
		"rrule:freq=daily;count=3",
	} {
		r, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if r.Freq() != Daily {
			t.Fatalf("Parse(%q): freq = %v", s, r.Freq())
		}
		if got := len(r.All()); got != 3 {
			t.Fatalf("Parse(%q): %d occurrences, want 3", s, got)
		}
	}
}

func TestStrToRRule(t *testing.T) {
	r, err := StrToRRule("DTSTART:19970902T090000\nRRULE:FREQ=YEARLY;COUNT=3")
	if err != nil {
		t.Fatal(err)
	}
	got := r.All()
	if len(got) != 3 || got[0].Year() != 1997 || got[2].Year() != 1999 {
		t.Fatalf("got %v", got)
	}
	if got[0].Hour() != 9 {
		t.Fatalf("time of day lost: %v", got[0])
	}
}

func TestStrToRRuleWithTZID(t *testing.T) {
	r, err := StrToRRule("DTSTART;TZID=America/New_York:19970902T090000\n" +
		"RRULE:FREQ=DAILY;COUNT=2")
	if err != nil {
		t.Skipf("TZID unavailable: %v", err)
	}
	got := r.All()
	if name := got[0].Location().String(); name != "America/New_York" {
		t.Fatalf("location = %q", name)
	}
	if got[0].Hour() != 9 {
		t.Fatalf("got %v", got[0])
	}
}

// TestUntilLocations covers both UNTIL forms: a UTC value keeps its instant, a
// naive one is read in DTSTART's zone.
func TestUntilLocations(t *testing.T) {
	utc, err := StrToRRule("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;UNTIL=19970904T090000Z")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(utc.All()); got != 3 {
		t.Fatalf("UTC until: %d occurrences, want 3", got)
	}

	ny, lerr := time.LoadLocation("America/New_York")
	if lerr != nil {
		t.Skip("no tz database")
	}
	naive, err := StrToRRule("DTSTART;TZID=America/New_York:19970902T090000\n" +
		"RRULE:FREQ=DAILY;UNTIL=19970904T090000")
	if err != nil {
		t.Fatal(err)
	}
	all := naive.All()
	if len(all) != 3 {
		t.Fatalf("naive until: %d occurrences, want 3", len(all))
	}
	if !all[2].Equal(time.Date(1997, 9, 4, 9, 0, 0, 0, ny)) {
		t.Fatalf("naive until: last occurrence %v", all[2])
	}
}

func TestParseLeniency(t *testing.T) {
	// dateutil accepts properties separated by spaces rather than newlines.
	r, err := StrToRRule(" DTSTART:19970902T090000 RRULE:FREQ=YEARLY;COUNT=3 ")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(r.All()); got != 3 {
		t.Fatalf("%d occurrences", got)
	}
}

func TestParseFolded(t *testing.T) {
	r, err := StrToRRule("DTSTART:19970902T090000\r\nRRULE:FREQ=YEARLY;COUNT=3;\r\n BYMONTH=1,2")
	if err != nil {
		t.Fatal(err)
	}
	got := r.All()
	if len(got) != 3 || got[0].Month() != time.January {
		t.Fatalf("folded rule: %v", got)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		{"FREQ=NEVER", ErrInvalidFreq},
		{"FREQ=DAILY;BOGUS=1", ErrParse},
		{"FREQ=DAILY;COUNT=x", ErrParse},
		{"FREQ=DAILY;BYDAY=XX", ErrInvalidWeekday},
		{"FREQ=DAILY;BYDAY=0MO", ErrInvalidWeekday},
		{"FREQ=DAILY;COUNT=1;COUNT=2", ErrParse},
		{"DTSTART:notadate\nRRULE:FREQ=DAILY", ErrParse},
		{"DTSTART:19970902T090000", ErrParse},
		{"", ErrParse},
	}
	for _, c := range cases {
		_, err := Parse(c.in)
		if !errors.Is(err, c.want) {
			t.Fatalf("Parse(%q): err = %v, want one wrapping %v", c.in, err, c.want)
		}
	}
	if _, err := StrToRRule("DTSTART:19970902T090000"); !errors.Is(err, ErrNoRRule) {
		t.Fatalf("StrToRRule without a rule: %v", err)
	}
	if _, err := StrToRRule("DTSTART:19970902T090000\nRRULE:FREQ=DAILY;COUNT=2\nEXDATE:19970903T090000"); !errors.Is(err, ErrParse) {
		t.Fatalf("StrToRRule on a set: %v", err)
	}
}

func TestRuleString(t *testing.T) {
	r := implNew(t, Options{Freq: Monthly, Count: 3, Interval: 2,
		ByWeekday: []Weekday{FR.Nth(-1)}, BySetPos: []int{1}, Wkst: SundayStart})
	want := "FREQ=MONTHLY;INTERVAL=2;WKST=SU;COUNT=3;BYSETPOS=1;BYDAY=-1FR"
	if got := r.RuleString(); got != want {
		t.Fatalf("RuleString() = %q, want %q", got, want)
	}
	if got, want := r.String(), "DTSTART:19970902T090000Z\nRRULE:"+want; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

// TestStringRoundTrip checks that serialization is re-parseable and stable.
func TestStringRoundTrip(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tz database")
	}
	rules := []Options{
		{Freq: Daily, Count: 10},
		{Freq: Weekly, Count: 4, Interval: 2, ByWeekday: []Weekday{TU, TH}, Wkst: SundayStart},
		{Freq: Monthly, Count: 5, ByWeekday: []Weekday{FR.Nth(-1)}},
		{Freq: Yearly, Count: 3, ByMonth: []int{1, 3}, ByMonthDay: []int{-1}},
		{Freq: Yearly, Count: 3, ByWeekNo: []int{20}, ByYearDay: []int{100}},
		{Freq: Hourly, Count: 6, ByHour: []int{9, 12}, ByMinute: []int{0, 30}},
		{Freq: Daily, Until: time.Date(1997, 12, 24, 0, 0, 0, 0, time.UTC)},
		{Freq: Daily, Count: 3, DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, ny)},
		{Freq: Monthly, Count: 4, BySetPos: []int{-1}, ByWeekday: []Weekday{MO, TU, WE, TH, FR}},
	}
	for _, o := range rules {
		r := implNew(t, o)
		text := r.String()
		back, err := StrToRRule(text)
		if err != nil {
			t.Fatalf("StrToRRule(%q): %v", text, err)
		}
		if back.String() != text {
			t.Fatalf("unstable serialization: %q then %q", text, back.String())
		}
		implEqual(t, text, back.All(), r.All())
	}
}

func TestStrToSet(t *testing.T) {
	s, err := StrToSet("DTSTART:19970902T090000Z\n" +
		"RRULE:FREQ=WEEKLY;COUNT=6;BYDAY=TU,TH\n" +
		"RDATE:19970905T090000Z\n" +
		"EXDATE:19970904T090000Z,19970911T090000Z")
	if err != nil {
		t.Fatal(err)
	}
	implEqual(t, "StrToSet", s.All(), implTimes(t,
		"1997-09-02 09:00:00", "1997-09-05 09:00:00",
		"1997-09-09 09:00:00", "1997-09-16 09:00:00", "1997-09-18 09:00:00"))
}
