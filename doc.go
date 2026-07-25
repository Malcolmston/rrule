// Package rrule is a standard-library-only implementation of RFC 5545
// recurrence rules and of the minimal iCalendar object model they live in.
//
// # Overview
//
// A recurrence rule describes an unbounded series of instants — "every second
// Tuesday", "the last Friday of every month", "every year on the first Sunday
// after the first Saturday of November" — in a few dozen characters of text.
// This package compiles such a rule, iterates it, composes rules into
// recurrence sets, and reads and writes the surrounding iCalendar object,
// using only the Go standard library. There are no third-party dependencies
// and no cgo.
//
// It is a re-implementation of the recurrence engines in Python's
// dateutil.rrule and in jkbrzt/rrule.js, and it complements the cron-style
// scheduling of the family's quartz package: cron cannot express "the last
// weekday of the month in even-numbered years", and RRULE cannot express a
// duty roster in seconds since boot.
//
// # Building a rule
//
// A rule is a Freq, an anchor, and any number of BYxxx rule parts:
//
//	r, err := rrule.New(rrule.Options{
//	        Freq:      rrule.Monthly,
//	        DTStart:   time.Date(1997, 9, 2, 9, 0, 0, 0, time.UTC),
//	        ByWeekday: []rrule.Weekday{rrule.FR.Nth(-1)},
//	        Count:     3,
//	})
//	r.All() // 1997-09-26, 1997-10-31, 1997-11-28, all at 09:00 UTC
//
// DTSTART anchors everything. Its date supplies the defaults RFC 5545 derives
// from FREQ — a MONTHLY rule with no BYxxx part recurs on DTSTART's day of the
// month, a WEEKLY one on DTSTART's weekday — its time of day supplies the
// defaults for BYHOUR, BYMINUTE and BYSECOND, and its time.Location is the
// location every occurrence is generated in.
//
// # How the engine works
//
// Iteration follows the structure RFC 5545 §3.3.10 describes and dateutil
// implements. For each interval of the base frequency the engine builds the
// set of candidate days, removes the days the BYxxx rule parts reject,
// applies BYSETPOS to the days that survived, and emits them in order:
//
//	FREQ=YEARLY   candidates: every day of the year
//	FREQ=MONTHLY  candidates: every day of the month
//	FREQ=WEEKLY   candidates: the seven days from the week start
//	FREQ=DAILY    and finer: the single current day
//
// A BYxxx part that is coarser than or equal to FREQ limits the candidate set;
// one that is finer expands it. BYWEEKDAY entries carrying an ordinal (-1FR,
// the last Friday) are only meaningful for MONTHLY and YEARLY and are handled
// separately from plain ones; for finer frequencies the ordinal is dropped, as
// the RFC requires. BYWEEKNO uses ISO 8601 week numbering relative to the
// rule's week start, and BYWEEKNO, BYYEARDAY and BYMONTHDAY all accept
// negative values counting back from the end of their period.
//
// # Week start
//
// WKST genuinely changes results: it moves the boundary of a FREQ=WEEKLY
// interval and it decides which days ISO week 1 contains. It is threaded
// through the engine rather than fixed at Monday. Because the zero value of
// Options.Wkst is time.Sunday but an unset week start must mean Monday — the
// RFC 5545 default — set Options.Wkst to the SundayStart constant, not to
// time.Sunday, to start weeks on Sunday.
//
// # Time zones and daylight saving
//
// Occurrences are generated in DTSTART's time.Location by wall-clock
// arithmetic, so a daily 09:00 rule stays at 09:00 across a daylight-saving
// transition rather than drifting to 08:00 or 10:00. Each occurrence is
// produced from a distinct set of wall-clock fields, so a transition can
// neither duplicate nor skip one. Where a wall clock does not exist — the hour
// a spring-forward transition removes — time.Date normalizes it forward, and
// where a wall clock is ambiguous the earlier of the two offsets is used.
//
// # Termination
//
// Rules can be impossible: FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2 never matches
// a day. Iteration therefore gives up after maxEmptyIterations consecutive
// intervals that produce nothing, and never iterates past year 9999. An
// impossible rule yields an empty result instead of hanging. All on a rule
// with neither COUNT nor UNTIL would be infinite, so it stops after
// MaxAllOccurrences occurrences; prefer a bounded rule, or use Between, After
// or Iterator.
//
// # Text
//
// Parse reads an RRULE line with or without its property name; StrToRRule
// reads a DTSTART + RRULE block; String writes that block back, stably and
// re-parseably; RuleString writes just the rule value that follows "RRULE:".
//
//	r, err := rrule.StrToRRule("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=10")
//
// # Recurrence sets
//
// Set composes a VEVENT's recurrence the way RFC 5545 does: any number of
// RRULE and RDATE inclusions minus any number of EXRULE and EXDATE
// exclusions. Inclusions are merged chronologically and de-duplicated, then
// exclusions are removed.
//
//	s := rrule.NewSet()
//	s.RRule(weekly)
//	s.ExDate(time.Date(1997, 9, 9, 9, 0, 0, 0, time.UTC))
//
// # iCalendar
//
// ParseCalendar reads a VCALENDAR into Calendar, Event and Property values,
// unfolding continuation lines per RFC 5545 §3.1, decoding property
// parameters such as TZID and VALUE=DATE, and turning RRULE, RDATE, EXRULE and
// EXDATE into a Set. Calendar.Encode writes the object back with CRLF line
// endings and content lines folded at 75 octets — folded by octet, never
// splitting a UTF-8 sequence — with TEXT values escaped per the RFC.
//
// # Errors
//
// All errors are wrapped sentinels; test them with errors.Is, e.g.
// errors.Is(err, rrule.ErrInvalidFreq) or errors.Is(err, rrule.ErrParse).
package rrule
