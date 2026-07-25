# API deviations

The package implements the shared contract signatures exactly. The notes below
record the places where the contract left a decision open, plus the small number
of additions made on top of it.

## Decisions the contract left open

### `Options.Wkst` and the `SundayStart` constant

The contract types the week start as `time.Weekday`, whose zero value is
`time.Sunday`. RFC 5545 defaults WKST to **Monday**, and almost every rule is
built without naming a week start, so reading the zero value as Sunday would
silently change the result of every `FREQ=WEEKLY;INTERVAL>1` and `BYWEEKNO`
rule.

The zero value therefore means "unset", i.e. Monday. To select Sunday
explicitly, use the exported constant:

```go
rrule.Options{Freq: rrule.Weekly, Interval: 2, Wkst: rrule.SundayStart}
```

`SundayStart` is `time.Weekday(7)`, which is congruent to Sunday modulo 7 and
cannot be confused with a real weekday. Parsing `WKST=SU` produces it, and
`String()` serializes it back to `WKST=SU`.

### `String()` emits a DTSTART line

`(*RRule).String()` returns two content lines:

```
DTSTART;TZID=America/New_York:19970902T090000
RRULE:FREQ=DAILY;COUNT=10
```

A rule is meaningless without its anchor, so a serialization that dropped
DTSTART would not round trip. This matches `str(rrule)` in dateutil, which also
emits the DTSTART line. `Parse` and `StrToRRule` both accept this form, as well
as a lone rule value with or without the `RRULE:` name. `RuleString()` (see
below) returns just the value when only the rule part is wanted.

### `All()` on an unbounded rule

A rule with neither COUNT nor UNTIL is infinite. `All` stops after
`MaxAllOccurrences` (10000) occurrences rather than never returning. The
contract asked for this to be bounded and documented rather than hanging.

### Termination guard

Iteration stops after 10000 consecutive frequency intervals that produce no
occurrence, and never runs past year 9999. This is what makes
`FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2` terminate with an empty result. The bound
is generous enough for legitimately sparse rules — `FREQ=DAILY;BYMONTH=2;
BYMONTHDAY=29` skips at most ~2920 days between occurrences.

### Daylight saving policy

Occurrences are generated from wall-clock fields in DTSTART's location, so a
daily 09:00 rule stays at 09:00 across a transition. A wall clock that a
spring-forward transition removed is resolved with the offset in effect before
the transition, which is what RFC 5545 §3.3.5 prescribes and what `time.Date`
does. Because that can make two wall clocks (02:00 and 03:00) name one instant
for an HOURLY rule crossing the gap, an occurrence identical to the one just
emitted is dropped, so no instant is ever produced twice and none is skipped.

## Additions

These are additive; nothing in the contract changed shape.

| Symbol | Why |
| --- | --- |
| `StrToSet(string) (*Set, error)` | `StrToRRule` only covers DTSTART + RRULE. A block that also carries RDATE, EXRULE or EXDATE is a recurrence set, which is what dateutil's `rrulestr` returns for the same input. `StrToRRule` rejects such a block with a message pointing here. |
| `(*RRule).RuleString() string` | The RRULE property value alone, for embedding in iCalendar output. |
| `(*RRule).Options() Options`, `.DTStart()`, `.Freq()` | Read back the compiled rule. |
| `(*Set).RRules()`, `.ExRules()`, `.RDates()`, `.ExDates()`, `.DTStart()`, `.Iterator()` | Read back a set's parts; `Iterator` mirrors `(*RRule).Iterator`. |
| `SundayStart` | See above. |
| `MaxAllOccurrences` | The documented bound on `All`. |
| `Freq.String()`, `Weekday.String()`, `Weekday.Nth()` | RFC 5545 spellings. |

## Known upstream disagreement

RFC 5545 §3.8.5.3 lists "every 3 hours from 9:00 AM to 5:00 PM on a specific
day" as

```
DTSTART;TZID=America/New_York:19970902T090000
RRULE:FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T170000Z
```

and says the result is 09:00, 12:00 and 15:00 local. `UNTIL=19970902T170000Z` is
13:00 in New York, so the normative reading gives 09:00 and 12:00 only; the
example's prose and its UNTIL do not agree. This package follows the normative
reading, and so does dateutil, which returns the same two occurrences.
