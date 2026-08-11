# rrule

RFC 5545 calendar recurrence for Go: parse, expand, and serialize `RRULE`
recurrence rules, compose them with `RDATE` / `EXRULE` / `EXDATE` into a
recurrence set, and read or write the surrounding iCalendar document. It is
built entirely on the Go standard library: no third-party modules, no cgo, no
`require` directives.

The reference implementations this port mirrors are `dateutil.rrule` (Python)
and `rrule.js`; the specification is
[RFC 5545 §3.3.10 and §3.8.5.3](https://www.rfc-editor.org/rfc/rfc5545.html#section-3.8.5.3).

## Features

- **All seven frequencies**: `Yearly`, `Monthly`, `Weekly`, `Daily`, `Hourly`,
  `Minutely`, `Secondly`, with `INTERVAL`, `COUNT`, and `UNTIL`.
- **Every RFC 5545 `BY…` part**: `BYMONTH`, `BYWEEKNO`, `BYYEARDAY`,
  `BYMONTHDAY`, `BYDAY`, `BYHOUR`, `BYMINUTE`, `BYSECOND`, and `BYSETPOS`.
  Negative values are supported throughout (`BYMONTHDAY=-1` for the last day of
  the month, `BYYEARDAY=-1`, `BYSETPOS=-2`).
- **Nth-weekday `BYDAY`**: `1FR`, `-1SU`, `20MO`, `-2MO`, expressed in Go as
  `FR.Nth(1)`, `SU.Nth(-1)`, and so on.
- **`BYSETPOS`** applied over the expansion set of each interval, so idioms like
  "the second-to-last weekday of the month"
  (`FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-2`) work as specified.
- **`WKST`**, which genuinely changes results for `FREQ=WEEKLY;INTERVAL>1` and
  for `BYWEEKNO` (ISO week numbering keyed off the week start). See
  [Gotchas](#gotchas) — selecting Sunday requires the `SundayStart` constant.
- **`Set`**: `RRULE` + `RDATE` + `EXRULE` + `EXDATE` merged into one ordered,
  de-duplicated occurrence stream. `StrToSet` parses a whole block into one,
  and `RRules` / `ExRules` / `RDates` / `ExDates` / `DTStart` read the parts
  back.
- **iCalendar object model**: `ParseCalendar` handles RFC 5545 line unfolding
  and property parameters; `Calendar.Encode` writes CRLF line endings with
  75-octet line folding.
- **Round-trippable serialization**: `RRule.String()` emits the `DTSTART` and
  `RRULE` lines together — a rule is meaningless without its anchor, so this is
  what round-trips through `Parse` / `StrToRRule` (and matches `str(rrule)` in
  dateutil). `RRule.RuleString()` returns just the rule value when you only want
  the property body.
- **Query API**: `All`, `Between`, `After`, `Before`, and a pull-based
  `Iterator` for streaming. `Options()`, `DTStart()`, and `Freq()` read a
  compiled rule back.
- Wrapped sentinel errors usable with `errors.Is`.

### Relationship to the family's `quartz` port

[`malcolmston/quartz`](https://github.com/malcolmston/quartz) schedules work on
cron expressions. Cron and RRULE are not interchangeable, and this library
exists because cron cannot express calendar recurrence:

- Cron has no notion of an **end** to a series — no `COUNT`, no `UNTIL`.
- Cron cannot say "**the third** Tuesday-or-Wednesday-or-Thursday of the month"
  — it has no `BYSETPOS`, so it cannot select the nth member of a computed set.
- Cron has no `INTERVAL`: "every other week" or "every 18 months" is not
  expressible, because cron matches on absolute field values rather than
  counting periods from a start date.
- Cron has no anchor date. RRULE is defined relative to `DTSTART`, which is what
  makes "every other week starting on this Monday" meaningful.
- Cron has no ISO week numbering (`BYWEEKNO`) or day-of-year (`BYYEARDAY`).

Use `quartz` to decide when a job runs; use `rrule` to decide when a *calendar
event* recurs, and to interoperate with anything that speaks iCalendar.

## Install

```sh
go get github.com/malcolmston/rrule
```

Requires Go 1.24 or newer.

## Usage

### From an RFC 5545 string

```go
r, err := rrule.StrToRRule(
    "DTSTART:19970902T090000Z\n" +
    "RRULE:FREQ=MONTHLY;COUNT=10;BYDAY=1FR")
if err != nil {
    // errors.Is(err, rrule.ErrInvalidRule), ...
}
for _, t := range r.All() {
    fmt.Println(t.Format(time.RFC3339))
}
```

`Parse` reads a bare rule line, with or without the `RRULE:` prefix:

```go
r, _ := rrule.Parse("FREQ=WEEKLY;INTERVAL=2;COUNT=8;WKST=SU;BYDAY=TU,TH")
```

### From typed options

```go
ny, _ := time.LoadLocation("America/New_York")

r, err := rrule.New(rrule.Options{
    Freq:      rrule.Monthly,
    DTStart:   time.Date(1997, 9, 4, 9, 0, 0, 0, ny),
    Count:     3,
    ByWeekday: []rrule.Weekday{rrule.TU, rrule.WE, rrule.TH},
    BySetPos:  []int{3},
})
// => 1997-09-04, 1997-10-07, 1997-11-06 at 09:00 local

fmt.Println(r.RuleString())
// FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3

fmt.Println(r.String())
// DTSTART;TZID=America/New_York:19970904T090000
// RRULE:FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3
```

### Nth weekdays

```go
// The last Friday of every other month.
r, _ := rrule.New(rrule.Options{
    Freq:      rrule.Monthly,
    Interval:  2,
    Count:     6,
    DTStart:   start,
    ByWeekday: []rrule.Weekday{rrule.FR.Nth(-1)},
})
```

### Windowed queries and streaming

```go
occ := r.Between(
    time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC),
    time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
    true, // inclusive
)

next := r.After(time.Now(), false)
prev := r.Before(time.Now(), false)

it := r.Iterator()
for {
    t, ok := it()
    if !ok {
        break
    }
    // ...
}
```

### Recurrence sets

```go
s := rrule.NewSet()
s.RRule(weekdaysRule)                 // every weekday
s.ExRule(holidayRule)                 // minus a recurring holiday
s.RDate(time.Date(2026, 7, 4, 9, 0, 0, 0, ny))  // plus a one-off
s.ExDate(time.Date(2026, 7, 3, 9, 0, 0, 0, ny)) // minus a one-off

for _, t := range s.Between(from, to, true) {
    fmt.Println(t)
}
fmt.Println(s.String()) // RRULE/EXRULE/RDATE/EXDATE lines
```

A block that carries more than `DTSTART` + `RRULE` is a recurrence set, so parse
it with `StrToSet` rather than `StrToRRule`:

```go
s, err := rrule.StrToSet(
    "DTSTART:19970902T090000Z\n" +
    "RRULE:FREQ=MONTHLY;COUNT=10;BYDAY=FR;BYMONTHDAY=13\n" +
    "EXDATE:19970902T090000Z")
```

(`StrToRRule` rejects such a block with an error pointing at `StrToSet`, rather
than silently dropping the `EXDATE`.)

### iCalendar

```go
cal, err := rrule.ParseCalendar(file) // unfolds RFC 5545 continuation lines
for _, ev := range cal.Events {
    if ev.RRule != nil {
        fmt.Println(ev.Summary, ev.RRule.String())
    }
}

var buf bytes.Buffer
_ = cal.Encode(&buf) // CRLF, folded at 75 octets
```

## Gotchas

### `Wkst: time.Sunday` does not mean Sunday — use `rrule.SundayStart`

`Options.Wkst` is a `time.Weekday`, and `time.Weekday`'s zero value is
`time.Sunday`. RFC 5545's default week start is **Monday**. Those two facts
collide: an `Options` literal that never mentions `Wkst` is indistinguishable
from one that sets `Wkst: time.Sunday`, so the zero value has to mean "unset",
i.e. Monday.

```go
// WRONG — this is the zero value, so it means "unset" and behaves as Monday.
rrule.Options{Freq: rrule.Weekly, Interval: 2, Wkst: time.Sunday}

// RIGHT — explicitly request a Sunday week start.
rrule.Options{Freq: rrule.Weekly, Interval: 2, Wkst: rrule.SundayStart}
```

`SundayStart` is `time.Weekday(7)` — congruent to Sunday modulo 7, and
impossible to confuse with a real `time.Weekday`. Parsing `WKST=SU` produces it,
and `String()` serializes it back to `WKST=SU`, so the string form has no such
ambiguity; this only bites when building `Options` by hand.

This matters because a wrong week start produces **subtly wrong occurrences
rather than an error**, in exactly two places:

- `FREQ=WEEKLY` with `INTERVAL` > 1, where `WKST` decides where each interval
  boundary falls. RFC 5545's own worked example makes the point: the same rule
  `FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU` from 1997-08-05 yields
  August 5, 10, 19, 24 under `WKST=MO` but August 5, 17, 19, 31 under `WKST=SU`.
  Both are in the parity corpus.
- `BYWEEKNO`, whose ISO week numbering is keyed off the week start.

Every other frequency ignores `WKST` entirely, so the bug is silent until
someone writes a biweekly or week-number rule.

## Parity

Parity is measured, not asserted. Two vendored, language-neutral corpora live in
`testdata/` and are replayed against the real exported API by
`upstream_parity_test.go`; `testdata/UPSTREAM.md` records source URLs, the
pinned dateutil commit, licensing, and the conversion method.

| Corpus | Cases | Passing | |
| --- | ---: | ---: | ---: |
| `dateutil/dateutil` — `tests/test_rrule.py` | 214 | 214 | **100.0%** |
| RFC 5545 §3.8.5.3 worked examples | 42 | 41 | **97.6%** |
| **Total** | **256** | **255** | **99.6%** |

Measured on 2026-07-24 with `go test -run TestUpstreamParityScore -v`. The
single failure is a defect in the RFC's own example text, described below; there
are no known behavioural gaps against dateutil.

The dateutil corpus was produced by an AST walk over `tests/test_rrule.py`,
converting every `self.assertEqual(list(rrule(...)), [datetime(...), ...])`
assertion into an `{id, rrule, dtstart, expected}` record: **214 converted, 26
skipped** out of 240 candidate assertions. The skips are, in full:

- **21** `byeaster=` cases — `BYEASTER` is a dateutil extension, not part of
  RFC 5545, and is deliberately out of scope.
- **2** cases passing Python arbitrary-precision integer expressions that have
  no literal RRULE spelling (`testLongIntegers`).
- **2** cases asserting Python `date`-vs-`datetime` type coercion rather than
  recurrence behaviour (`testUntilWithDate`, `testDTStartIsDate`).
- **1** case with a sub-second `DTSTART`; RFC 5545 has no sub-second resolution
  (`testDTStartWithMicroseconds`).

Every one of the 214 converted cases was verified to round-trip through
dateutil's own `rrulestr()` before being committed, so a failure here is a
failure of this port, never a transcription error.

Cases this port does not yet reproduce are listed in the `knownGaps` map in
`upstream_parity_test.go`, each with a reason. `knownGaps` keeps CI green but
does **not** inflate the score: `TestUpstreamParityScore` counts allowlisted
cases as failures and logs the honest number, which is what `parity.json`
reports.

One RFC 5545 example is expected to fail permanently and is recorded as such:
`every-3-hours-from-9am-to-5pm-on-a-specific-day` states occurrences of
09:00/12:00/15:00 EDT but writes `UNTIL=19970902T170000Z`, which is 13:00 EDT
and truncates the series at 12:00. That is
[RFC 5545 errata ID 3779](https://www.rfc-editor.org/errata/eid3779); dateutil
fails it too. The example is transcribed faithfully rather than silently
corrected.

## Time zones and DST

Occurrences are generated in `DTStart`'s `time.Location` and preserve **wall
clock** time, which is what RFC 5545 requires: a 09:00 daily meeting stays at
09:00 across a DST transition, even though the UTC instants shift by an hour.
The RFC 5545 corpus is anchored at `TZID=America/New_York` specifically so this
behaviour is under test across both spring-forward and fall-back.

Three consequences worth knowing:

- `UNTIL` is compared as an **instant**. RFC 5545 requires `UNTIL` to be UTC
  when `DTSTART` is zoned, and this port follows that rule.
- A wall clock that occurs **twice** — the hour a fall-back transition repeats —
  refers to the first of the two, i.e. the offset in effect before the
  transition. That is RFC 5545 §3.3.5, and it is what `time.Date` does.
- A wall clock that does **not occur** — the hour a spring-forward transition
  removes — produces no occurrence at all. RFC 5545 §3.3.10 says such instances
  "MUST be ignored and MUST NOT be counted as part of the recurrence set", so a
  daily 02:30 rule has no instance on the day its zone skips 02:30, and a
  `COUNT` is not spent on it. This is also what keeps the stream strictly
  increasing: `time.Date` renders a nonexistent wall clock at an *earlier*
  instant, so an `FREQ=HOURLY` rule stepping through the gap would otherwise
  emit an instant it had already produced, out of order.

Occurrences are therefore always strictly increasing, and no instant is ever
produced twice — which is what `Between`, `After` and `Before` need, since each
is a single forward pass.

## Unbounded rules

An RRULE with neither `COUNT` nor `UNTIL` describes an infinite series.

- `AllLimit(n)` is the bounded form of `All()` and never returns — or
  allocates — more than `n` occurrences. Reach for it whenever the rule text
  came from somewhere you do not control: a `COUNT` or an `UNTIL` makes a rule
  finite but not necessarily small, and `FREQ=SECONDLY;UNTIL=99991231T235959Z`
  is a perfectly valid rule with some 2.5e11 occurrences that `All()` would
  faithfully try to materialize. `Between` and `Iterator` are bounded the same
  way, by an argument you supply.
- `INTERVAL` is capped at 1000000 for the same reason. A larger value cannot
  place a second occurrence before the engine's year-9999 ceiling under any
  frequency, and used to overflow the day arithmetic into a negative
  day-of-year index. `New` rejects it.
- `All()` never hangs: on an unbounded rule it stops after
  `MaxAllOccurrences` (10000) occurrences and returns what it has. Treat a
  result of exactly `MaxAllOccurrences` as "probably truncated" — for rules that
  legitimately run forever, prefer `Between`, `After`, or `Iterator`, which are
  the streaming entry points and impose no cap.
- Iteration is bounded internally, so an **impossible** rule
  (`FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2`, or a `BYWEEKNO=53` in a year that has
  only 52) gives up after a bounded search instead of spinning forever. This is
  the same defence dateutil applies, and it means no input can hang a caller.
- The parity harness enforces this from the outside as well: every upstream case
  runs on its own goroutine under a timeout, so a hang is reported as a test
  failure rather than wedging CI.

## License

MIT — see [LICENSE](LICENSE).

## A note on provenance

This is an independent, clean-room re-implementation written against RFC 5545.
It is not affiliated with, endorsed by, or derived from the source code of
[dateutil](https://github.com/dateutil/dateutil) or
[rrule.js](https://github.com/jkbrzt/rrule). Only the *expected values* of
dateutil's published test assertions are reproduced, restated as JSON data, for
conformance testing.
