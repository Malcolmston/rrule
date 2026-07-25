# Upstream conformance corpora

Both files in this directory are vendored, language-neutral restatements of
existing RRULE conformance material. They are data, not code: each case is an
RFC 5545 `RRULE` string, a `DTSTART`, and the exact list of instants the
upstream implementation (or the specification's own prose) produces.

Retrieved: **2026-07-24**.

## `dateutil-rrule-cases.json`

- **Source:** `tests/test_rrule.py` from [dateutil/dateutil](https://github.com/dateutil/dateutil)
- **Raw URL:** <https://raw.githubusercontent.com/dateutil/dateutil/master/tests/test_rrule.py>
- **Commit:** `48bd1af97e71baf8e96fce5b663d589caac8f147` (`master` as of 2026-07-24)
- **License:** dateutil is dual-licensed Apache-2.0 / BSD-3-Clause. Copyright
  Gustavo Niemeyer and the dateutil contributors. Only the *expected values* of
  its test assertions are reproduced here, restated as JSON; no dateutil source
  code is vendored or executed.

### How the conversion was done

A throwaway Python script (not kept in this repository) parsed `test_rrule.py`
with the standard `ast` module — no regexes, no execution of the test file —
and walked every `def test*` method looking for assertions of exactly the shape

```python
self.assertEqual(list(rrule(FREQ, count=N, byxxx=..., dtstart=datetime(...))),
                 [datetime(...), datetime(...), ...])
```

For each match it rebuilt the equivalent RFC 5545 `RRULE` string from the
keyword arguments:

| dateutil kwarg | RRULE part |
| --- | --- |
| positional `FREQ` constant | `FREQ=` |
| `interval` | `INTERVAL=` |
| `wkst` | `WKST=` |
| `count` | `COUNT=` |
| `until` | `UNTIL=` (basic format, `Z`) |
| `bymonth`, `bymonthday`, `byyearday`, `byweekno`, `byhour`, `byminute`, `bysecond`, `bysetpos` | the identically named `BY…` part |
| `byweekday` | `BYDAY=` — `TU` → `TU`, `TU(1)` → `1TU`, `TH(-1)` → `-1TH`, integer `0..6` → `MO..SU` |

`datetime(...)` literals are naive in the upstream tests; they are emitted as
UTC (`…Z`) instants, which is faithful because dateutil's naive arithmetic is
wall-clock arithmetic and UTC has no offset transitions. Where one test method
contains several qualifying assertions, later ones get a `#2`, `#3`, … suffix on
the case ID so every ID is unique and traceable back to its method.

### Counts

- 240 candidate `assertEqual(list(rrule(...)), [...])` sites found
- **214 converted**
- **26 skipped**, by reason:

| Skipped | Count | Why |
| --- | --- | --- |
| `byeaster=` | 21 | `BYEASTER` is a dateutil extension, not part of RFC 5545, and is out of scope for this port. |
| non-literal integer arguments | 2 | `testLongIntegers` passes Python arbitrary-precision `long` expressions that have no literal RRULE spelling. |
| `date` instead of `datetime` | 2 | `testUntilWithDate`, `testDTStartIsDate` assert Python type-coercion behaviour, not recurrence behaviour. |
| sub-second `DTSTART` | 1 | `testDTStartWithMicroseconds` — RFC 5545 has no sub-second resolution. |

Assertions in `test_rrule.py` that are not of the `list(rrule(...))` form
(`rruleset` composition, `rrulestr` round-trips, `.between()` / `.before()` /
`.after()` probes, `str()` serialization checks, error-raising cases, and the
`freeze_time`/`tz`-aware suites) are outside this corpus by construction; they
are covered — where in scope — by the port's own unit tests.

## `rfc5545-examples.json`

- **Source:** RFC 5545 §3.8.5.3 "Recurrence Rule", the worked
  `RRULE … ==> dates` examples
- **URL:** <https://www.rfc-editor.org/rfc/rfc5545.txt>
- **Copyright:** © 2009 IETF Trust and the persons identified as the document
  authors. Reproduced here as short factual excerpts for interoperability
  testing under the terms of BCP 78.

All 42 examples in that section were transcribed by hand from the prose. Every
one of them uses `DTSTART;TZID=America/New_York`, so each case carries a
`"tzid"` field and its timestamps are written with the correct UTC offset for
that instant — which makes this corpus a real DST test as well as a recurrence
test. Where the RFC writes an expansion in shorthand ("September 2-30;October
1-25", "9:00,9:20,9:40 … 16:40"), the expansion was generated mechanically.

Two shapes needed extra fields:

- `"limit": N` — the RFC example is explicitly "forever" (`…`). Only the first
  `N` occurrences stated in the prose are compared, read from a bounded window
  so an infinite rule still terminates.
- `"exdate": [...]` — `every-friday-the-13th-forever` is specified in the RFC
  with an accompanying `EXDATE`, so it is replayed through `Set`.

### Known defect in the source material

`every-3-hours-from-9am-to-5pm-on-a-specific-day` is transcribed faithfully and
is expected to fail: the example's `UNTIL=19970902T170000Z` is 13:00 EDT, which
contradicts its own stated occurrences of 09:00/12:00/15:00 local. This is
[RFC 5545 errata ID 3779]. It is recorded in `knownGaps` in
`upstream_parity_test.go` rather than silently corrected, because correcting the
corpus would mean testing against something the RFC does not say.

## Attribution

This repository is an independent re-implementation. It is not affiliated with,
endorsed by, or derived from the source code of dateutil, rrule.js, or the IETF.
