package rrule

import "errors"

// Sentinel errors returned by the package. Every error returned by rrule
// wraps one of these, so callers may classify failures with errors.Is:
//
//	_, err := rrule.Parse("FREQ=NEVER")
//	errors.Is(err, rrule.ErrInvalidFreq) // true
var (
	// ErrInvalidRule indicates a rule whose options are inconsistent or out of
	// range (a bad INTERVAL, a BYMONTH of 13, a BYSETPOS of 0, ...).
	ErrInvalidRule = errors.New("rrule: invalid recurrence rule")

	// ErrInvalidFreq indicates a missing or unrecognized FREQ rule part.
	ErrInvalidFreq = errors.New("rrule: invalid frequency")

	// ErrInvalidWeekday indicates a BYDAY entry that is not a two-letter
	// weekday, optionally prefixed by a non-zero ordinal (for example "-1FR").
	ErrInvalidWeekday = errors.New("rrule: invalid weekday")

	// ErrEmptyRule indicates a rule whose FREQ, INTERVAL and same-level BYxxx
	// parts can never agree, so the rule can never produce an occurrence (for
	// example FREQ=HOURLY;INTERVAL=2 starting at an odd hour with BYHOUR=1).
	ErrEmptyRule = errors.New("rrule: rule generates an empty set")

	// ErrParse indicates malformed RFC 5545 text: an unknown rule part, a value
	// that is not a number, or a date-time that does not match the RFC 5545
	// forms.
	ErrParse = errors.New("rrule: parse error")

	// ErrNoRRule indicates that a block passed to StrToRRule contained no
	// RRULE property and no bare rule value.
	ErrNoRRule = errors.New("rrule: no RRULE found")

	// ErrInvalidCalendar indicates malformed iCalendar input: a missing or
	// mismatched BEGIN/END pair, or a content line without a name.
	ErrInvalidCalendar = errors.New("rrule: invalid iCalendar data")
)
