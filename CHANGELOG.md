# Changelog

All notable changes to this project are documented in this file. The format is
loosely based on [Keep a Changelog](https://keepachangelog.com/), and the
project aims to follow semantic versioning.

## [0.1.0] - 2026-07-24

### Added

- Initial release: an RFC 5545 recurrence-rule engine for Go, built entirely on
  the standard library.
- **`RRule`** with all seven frequencies (`Yearly`, `Monthly`, `Weekly`,
  `Daily`, `Hourly`, `Minutely`, `Secondly`), `Interval`, `Count`, `Until`,
  `Wkst`, and every RFC 5545 `BY…` part: `BYMONTH`, `BYWEEKNO`, `BYYEARDAY`,
  `BYMONTHDAY`, `BYDAY` (including nth-weekday forms such as `-1FR`), `BYHOUR`,
  `BYMINUTE`, `BYSECOND`, and `BYSETPOS`.
- Construction from a typed `Options` struct (`New`), from a bare RRULE line
  (`Parse`), or from a full `DTSTART` + `RRULE` block (`StrToRRule`).
  `RRule.String` round-trips back to RFC 5545 form.
- Query API: `All`, `Between`, `After`, `Before`, and a pull-based `Iterator`.
- **`Set`** composing `RRULE`, `RDATE`, `EXRULE`, and `EXDATE` into a single
  merged, de-duplicated, ordered occurrence stream.
- **iCalendar object model**: `ParseCalendar` (with RFC 5545 line unfolding)
  and `Calendar.Encode` (75-octet folding, CRLF line endings), `Event` with
  `RRULE`/`RDATE`/`EXDATE` wiring and all-day handling.
- Location-aware recurrence: occurrences are generated in `DTSTART`'s
  `time.Location`, so wall-clock times are preserved across DST transitions.
- Bounded iteration: impossible rules (for example
  `FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2`) give up after a bounded search rather
  than spinning forever.
- Wrapped sentinel errors for use with `errors.Is`.
- **Upstream parity harness**: 214 cases converted from dateutil's
  `tests/test_rrule.py` plus all 42 worked examples from RFC 5545 §3.8.5.3,
  vendored as language-neutral JSON in `testdata/` and replayed by
  `upstream_parity_test.go`. See `testdata/UPSTREAM.md`.
