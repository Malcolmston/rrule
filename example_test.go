package rrule_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/rrule"
)

// implExampleStart is the anchor the examples share.
var implExampleStart = time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC)

func ExampleNew() {
	r, err := rrule.New(rrule.Options{
		Freq:      rrule.Monthly,
		DTStart:   implExampleStart,
		ByWeekday: []rrule.Weekday{rrule.FR.Nth(-1)},
		Count:     3,
	})
	if err != nil {
		panic(err)
	}
	for _, t := range r.All() {
		fmt.Println(t.Format("Mon 2006-01-02 15:04"))
	}
	// Output:
	// Fri 1997-09-26 09:00
	// Fri 1997-10-31 09:00
	// Fri 1997-11-28 09:00
}

func ExampleStrToRRule() {
	r, err := rrule.StrToRRule("DTSTART:19970902T090000Z\n" +
		"RRULE:FREQ=WEEKLY;INTERVAL=2;COUNT=4;WKST=SU;BYDAY=TU,TH")
	if err != nil {
		panic(err)
	}
	for _, t := range r.All() {
		fmt.Println(t.Format("Mon 2006-01-02"))
	}
	fmt.Println(r.String())
	// Output:
	// Tue 1997-09-02
	// Thu 1997-09-04
	// Tue 1997-09-16
	// Thu 1997-09-18
	// DTSTART:19970902T090000Z
	// RRULE:FREQ=WEEKLY;INTERVAL=2;WKST=SU;COUNT=4;BYDAY=TU,TH
}

func ExampleSet() {
	weekly, err := rrule.New(rrule.Options{
		Freq:      rrule.Weekly,
		DTStart:   implExampleStart,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.TH},
		Count:     6,
	})
	if err != nil {
		panic(err)
	}
	s := rrule.NewSet()
	s.RRule(weekly)
	s.ExDate(time.Date(1997, 9, 4, 9, 0, 0, 0, time.UTC))
	s.RDate(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC))
	for _, t := range s.All() {
		fmt.Println(t.Format("Mon 2006-01-02"))
	}
	// Output:
	// Tue 1997-09-02
	// Fri 1997-09-05
	// Tue 1997-09-09
	// Thu 1997-09-11
	// Tue 1997-09-16
	// Thu 1997-09-18
}

func ExampleCalendar_Encode() {
	r, err := rrule.New(rrule.Options{Freq: rrule.Daily, DTStart: implExampleStart, Count: 3})
	if err != nil {
		panic(err)
	}
	cal := &rrule.Calendar{
		ProdID: "-//Example Corp.//Scheduler//EN",
		Events: []*rrule.Event{{
			UID:     "1@example.com",
			Summary: "Stand-up; short",
			DTStart: implExampleStart,
			RRule:   r,
		}},
	}
	var b strings.Builder
	if err := cal.Encode(&b); err != nil {
		panic(err)
	}
	fmt.Print(strings.ReplaceAll(b.String(), "\r\n", "\n"))
	// Output:
	// BEGIN:VCALENDAR
	// VERSION:2.0
	// PRODID:-//Example Corp.//Scheduler//EN
	// BEGIN:VEVENT
	// UID:1@example.com
	// DTSTART:19970902T090000Z
	// SUMMARY:Stand-up\; short
	// RRULE:FREQ=DAILY;COUNT=3
	// END:VEVENT
	// END:VCALENDAR
}

func ExampleParseCalendar() {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:1@example.com\r\nDTSTART:19970902T090000Z\r\n" +
		"SUMMARY:Weekly review\r\nRRULE:FREQ=WEEKLY;COUNT=3\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := rrule.ParseCalendar(strings.NewReader(ics))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	ev := cal.Events[0]
	fmt.Println(ev.Summary)
	for _, t := range ev.RRule.All() {
		fmt.Println(t.Format("2006-01-02"))
	}
	// Output:
	// Weekly review
	// 1997-09-02
	// 1997-09-09
	// 1997-09-16
}
