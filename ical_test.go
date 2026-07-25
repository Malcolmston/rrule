package rrule

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const implSampleCalendar = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Example Corp.//CalendarApp 1.0//EN\r\n" +
	"BEGIN:VTIMEZONE\r\n" +
	"TZID:America/New_York\r\n" +
	"END:VTIMEZONE\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:19970901T130000Z-123401@example.com\r\n" +
	"DTSTART;TZID=America/New_York:19970902T090000\r\n" +
	"DTEND;TZID=America/New_York:19970902T100000\r\n" +
	"SUMMARY:Annual Employee Review\r\n" +
	"DESCRIPTION:Bring the report\\, the slides\\; and a pen.\\nSecond line.\r\n" +
	"LOCATION:Room 3\\, Building B\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=6;BYDAY=TU,TH\r\n" +
	"EXDATE;TZID=America/New_York:19970904T090000\r\n" +
	"CATEGORIES:BUSINESS,HUMAN RESOURCES\r\n" +
	"BEGIN:VALARM\r\n" +
	"ACTION:DISPLAY\r\n" +
	"END:VALARM\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseCalendar(t *testing.T) {
	cal, err := ParseCalendar(strings.NewReader(implSampleCalendar))
	if err != nil {
		t.Fatal(err)
	}
	if cal.Version != "2.0" || cal.ProdID != "-//Example Corp.//CalendarApp 1.0//EN" {
		t.Fatalf("calendar header = %q / %q", cal.Version, cal.ProdID)
	}
	if len(cal.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(cal.Events))
	}
	ev := cal.Events[0]
	if ev.UID != "19970901T130000Z-123401@example.com" {
		t.Fatalf("UID = %q", ev.UID)
	}
	if ev.Summary != "Annual Employee Review" {
		t.Fatalf("SUMMARY = %q", ev.Summary)
	}
	if want := "Bring the report, the slides; and a pen.\nSecond line."; ev.Description != want {
		t.Fatalf("DESCRIPTION = %q, want %q", ev.Description, want)
	}
	if want := "Room 3, Building B"; ev.Location != want {
		t.Fatalf("LOCATION = %q, want %q", ev.Location, want)
	}
	if ev.AllDay {
		t.Fatal("event wrongly marked all-day")
	}
	if name := ev.DTStart.Location().String(); name != "America/New_York" {
		t.Fatalf("DTSTART location = %q", name)
	}
	if ev.DTEnd.Sub(ev.DTStart) != time.Hour {
		t.Fatalf("DTEND - DTSTART = %v", ev.DTEnd.Sub(ev.DTStart))
	}
	if ev.RRule == nil || ev.Set == nil {
		t.Fatal("recurrence not decoded")
	}
	// The EXDATE removes the 4th, so six rule occurrences yield five.
	got := ev.Set.All()
	if len(got) != 5 || got[1].Day() != 9 {
		t.Fatalf("recurrence set = %v", got)
	}
	// Unmodelled properties survive verbatim.
	if p := ev.Props["CATEGORIES"]; len(p) != 1 || p[0].Value != "BUSINESS,HUMAN RESOURCES" {
		t.Fatalf("CATEGORIES = %v", ev.Props["CATEGORIES"])
	}
	// A nested component's properties do not leak into the event.
	if _, ok := ev.Props["ACTION"]; ok {
		t.Fatal("VALARM property leaked into the event")
	}
	if _, ok := ev.Props["TZID"]; ok {
		t.Fatal("VTIMEZONE property leaked into the event")
	}
}

func TestParseCalendarUnfolding(t *testing.T) {
	const long = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:1\r\n" +
		"DTSTART:19970902T090000Z\r\n" +
		"SUMMARY:This description is long enough that a sane encoder woul\r\n" +
		" d have folded it,\r\n\tand it may be continued with a tab too.\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ParseCalendar(strings.NewReader(long))
	if err != nil {
		t.Fatal(err)
	}
	want := "This description is long enough that a sane encoder would have folded it," +
		"and it may be continued with a tab too."
	if got := cal.Events[0].Summary; got != want {
		t.Fatalf("unfolded SUMMARY = %q, want %q", got, want)
	}
}

func TestParseCalendarAllDay(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:2\r\nDTSTART;VALUE=DATE:19970902\r\nDTEND;VALUE=DATE:19970903\r\n" +
		"SUMMARY:Holiday\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ParseCalendar(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	ev := cal.Events[0]
	if !ev.AllDay {
		t.Fatal("all-day event not detected")
	}
	if ev.DTStart.Hour() != 0 || ev.DTStart.Day() != 2 {
		t.Fatalf("DTSTART = %v", ev.DTStart)
	}
}

func TestParseCalendarQuotedParameter(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:3\r\nDTSTART:19970902T090000Z\r\n" +
		"ATTENDEE;CN=\"Doe, John: chair\";ROLE=CHAIR:mailto:j@example.com\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ParseCalendar(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	p := cal.Events[0].Props["ATTENDEE"][0]
	if got := p.Params["CN"]; len(got) != 1 || got[0] != "Doe, John: chair" {
		t.Fatalf("CN = %q", p.Params["CN"])
	}
	if got := p.Params["ROLE"]; len(got) != 1 || got[0] != "CHAIR" {
		t.Fatalf("ROLE = %q", p.Params["ROLE"])
	}
	if p.Value != "mailto:j@example.com" {
		t.Fatalf("value = %q", p.Value)
	}
}

func TestParseCalendarErrors(t *testing.T) {
	for _, src := range []string{
		"BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nEND:VCALENDAR\r\n",
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"BEGIN:VCALENDAR\r\nnoname\r\nEND:VCALENDAR\r\n",
	} {
		if _, err := ParseCalendar(strings.NewReader(src)); !errors.Is(err, ErrInvalidCalendar) {
			t.Fatalf("ParseCalendar(%q): err = %v, want one wrapping ErrInvalidCalendar", src, err)
		}
	}
}

// implEncode renders a calendar or fails the test.
func implEncode(t *testing.T, c *Calendar) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestEncodeFoldsAt75Octets(t *testing.T) {
	cal := &Calendar{Events: []*Event{{
		UID:     "fold-1",
		DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC),
		Summary: strings.Repeat("abcdefghij", 30),
	}}}
	out := implEncode(t, cal)
	if !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Fatalf("output does not end with a CRLF-terminated END: %q", out[len(out)-20:])
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Fatalf("line of %d octets: %q", len(line), line)
		}
	}
	cal2, err := ParseCalendar(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := cal2.Events[0].Summary; got != cal.Events[0].Summary {
		t.Fatalf("summary did not survive folding: %q", got)
	}
}

// TestEncodeFoldingKeepsUTF8 checks that folding counts octets but never cuts
// a multi-byte rune in half.
func TestEncodeFoldingKeepsUTF8(t *testing.T) {
	summary := strings.Repeat("é☃", 60) // two and three octets per rune
	cal := &Calendar{Events: []*Event{{
		UID:     "fold-2",
		DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC),
		Summary: summary,
	}}}
	out := implEncode(t, cal)
	for _, line := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Fatalf("line of %d octets: %q", len(line), line)
		}
		if !utf8.ValidString(line) {
			t.Fatalf("folding split a UTF-8 sequence: %q", line)
		}
	}
	cal2, err := ParseCalendar(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := cal2.Events[0].Summary; got != summary {
		t.Fatalf("summary did not survive folding: %q", got)
	}
}

func TestEncodeEscapesText(t *testing.T) {
	cal := &Calendar{Events: []*Event{{
		UID:         "esc",
		DTStart:     time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC),
		Summary:     `a, b; c\ d`,
		Description: "line one\nline two",
	}}}
	out := implEncode(t, cal)
	if !strings.Contains(out, `SUMMARY:a\, b\; c\\ d`) {
		t.Fatalf("SUMMARY not escaped: %q", out)
	}
	if !strings.Contains(out, `DESCRIPTION:line one\nline two`) {
		t.Fatalf("DESCRIPTION not escaped: %q", out)
	}
	cal2, err := ParseCalendar(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := cal2.Events[0].Summary; got != cal.Events[0].Summary {
		t.Fatalf("SUMMARY round trip = %q", got)
	}
	if got := cal2.Events[0].Description; got != cal.Events[0].Description {
		t.Fatalf("DESCRIPTION round trip = %q", got)
	}
}

func TestCalendarRoundTrip(t *testing.T) {
	cal, err := ParseCalendar(strings.NewReader(implSampleCalendar))
	if err != nil {
		t.Fatal(err)
	}
	out := implEncode(t, cal)
	again, err := ParseCalendar(strings.NewReader(out))
	if err != nil {
		t.Fatalf("re-parsing %q: %v", out, err)
	}
	a, b := cal.Events[0], again.Events[0]
	if a.UID != b.UID || a.Summary != b.Summary || a.Description != b.Description || a.Location != b.Location {
		t.Fatalf("text properties changed: %+v vs %+v", a, b)
	}
	if !a.DTStart.Equal(b.DTStart) || !a.DTEnd.Equal(b.DTEnd) {
		t.Fatalf("times changed: %v/%v vs %v/%v", a.DTStart, a.DTEnd, b.DTStart, b.DTEnd)
	}
	implEqual(t, "calendar round trip", b.Set.All(), a.Set.All())
	if !strings.Contains(out, "DTSTART;TZID=America/New_York:19970902T090000") {
		t.Fatalf("TZID lost on encode: %q", out)
	}
	if !strings.Contains(out, "EXDATE:") {
		t.Fatalf("EXDATE lost on encode: %q", out)
	}
}

func TestEncodeAllDay(t *testing.T) {
	cal := &Calendar{Events: []*Event{{
		UID:     "allday",
		AllDay:  true,
		DTStart: time.Date(1997, 9, 2, 0, 0, 0, 0, time.UTC),
		DTEnd:   time.Date(1997, 9, 3, 0, 0, 0, 0, time.UTC),
	}}}
	out := implEncode(t, cal)
	if !strings.Contains(out, "DTSTART;VALUE=DATE:19970902\r\n") {
		t.Fatalf("all-day DTSTART = %q", out)
	}
	again, err := ParseCalendar(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Events[0].AllDay {
		t.Fatal("all-day flag lost")
	}
}

// TestUnfold covers the raw line-unfolding helper, including a bare LF and a
// tab continuation.
func TestUnfold(t *testing.T) {
	got := unfold("A:1\r\nB:2\r\n 3\nC:4\n\t5\r\n")
	want := []string{"A:1", "B:23", "C:45", ""}
	if len(got) != len(want) {
		t.Fatalf("unfold gave %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unfold[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
