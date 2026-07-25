package rrule

// Upstream-parity tests. Every case in this file is a known-answer vector
// mechanically vendored from one of the two authoritative RRULE conformance
// corpora, and replayed against this package's real, exported Go API:
//
//   - dateutil/dateutil, tests/test_rrule.py — the reference Python
//     implementation this package mirrors. Each
//     `self.assertEqual(list(rrule(...)), [datetime(...), ...])` assertion was
//     converted by an AST walk into a language-neutral JSON case
//     (testdata/dateutil-rrule-cases.json).
//   - RFC 5545 §3.8.5.3 — the specification's own worked "RRULE ==> dates"
//     examples, transcribed from the prose into the same shape
//     (testdata/rfc5545-examples.json). These carry TZID=America/New_York and
//     therefore also exercise DST behaviour.
//
// See testdata/UPSTREAM.md for source URLs, the pinned dateutil commit, the
// retrieval date, licensing, and exactly which upstream cases were skipped.
//
// Comparisons are always made with time.Time.Equal against instants, never
// against formatted strings, so a correct result in a different but equivalent
// location still passes. Cases that this port does not yet reproduce are listed
// in knownGaps so CI stays green while the true score is measured and logged.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// caseTimeout bounds each individual upstream case. RRULE iteration can be
// made to spin forever by impossible rules (FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2),
// so every case runs on its own goroutine and a hang is reported as a failure
// rather than wedging the test binary.
const caseTimeout = 10 * time.Second

// upstreamCase is one vendored conformance vector. It is deliberately
// language-neutral: an RFC 5545 RRULE string, a DTSTART, and the exact list of
// instants the upstream implementation (or the RFC prose) produces.
type upstreamCase struct {
	ID       string   `json:"id"`
	RRule    string   `json:"rrule"`
	TZID     string   `json:"tzid"`
	DTStart  string   `json:"dtstart"`
	Expected []string `json:"expected"`
	// Limit, when non-zero, marks an unbounded ("forever") rule: only the
	// first Limit occurrences are compared, taken from a bounded window.
	Limit int `json:"limit"`
	// ExDate holds RFC 5545 EXDATE instants; a case with EXDATE is replayed
	// through Set rather than RRule.
	ExDate []string `json:"exdate"`
}

// knownGaps lists upstream case IDs this port does not yet reproduce, mapped to
// the reason. Entries keep CI green; the measured parity score reported by
// TestUpstreamParityScore counts them as failures regardless, so the score is
// never inflated by this allowlist.
var knownGaps = map[string]string{
	// RFC 5545 erratum: the prose says 09:00, 12:00, 15:00 local (EDT) but
	// UNTIL=19970902T170000Z is 13:00 EDT, which cuts the series at 12:00.
	// Both dateutil and this port honour UNTIL, so the example's own stated
	// output is unreachable. See RFC Errata ID 3779.
	"rfc5545/every-3-hours-from-9am-to-5pm-on-a-specific-day": "RFC 5545 erratum 3779: UNTIL in the example contradicts its stated occurrences",
}

func loadCorpus(t *testing.T, path string) []upstreamCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cases []upstreamCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s: empty corpus", path)
	}
	return cases
}

// parseInstant decodes an RFC 3339 timestamp from the corpus, rendered in loc
// when the case carries a TZID so that DTSTART is a genuine local time.
func parseInstant(s string, loc *time.Location) (time.Time, error) {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return ts.In(loc), nil
}

// dtstartLine renders the case's DTSTART as an RFC 5545 content line, with a
// TZID parameter for zoned cases and a "Z" suffix for UTC ones.
func dtstartLine(c upstreamCase, start time.Time) string {
	if c.TZID != "" {
		return fmt.Sprintf("DTSTART;TZID=%s:%s", c.TZID, start.Format("20060102T150405"))
	}
	return "DTSTART:" + start.UTC().Format("20060102T150405") + "Z"
}

// buildOccurrences constructs the rule described by c and returns the instants
// it generates, using All for bounded rules and a bounded Between window for
// rules the upstream corpus marks as running forever.
func buildOccurrences(c upstreamCase, start time.Time, want []time.Time, loc *time.Location) ([]time.Time, error) {
	block := dtstartLine(c, start) + "\n" + "RRULE:" + c.RRule
	r, err := StrToRRule(block)
	if err != nil {
		return nil, fmt.Errorf("StrToRRule(%q): %w", block, err)
	}

	var (
		all   func() []time.Time
		bound func(after, before time.Time) []time.Time
	)
	if len(c.ExDate) > 0 {
		s := NewSet()
		s.RRule(r)
		for _, ex := range c.ExDate {
			ts, err := parseInstant(ex, loc)
			if err != nil {
				return nil, fmt.Errorf("exdate %q: %w", ex, err)
			}
			s.ExDate(ts)
		}
		all = s.All
		bound = func(a, b time.Time) []time.Time { return s.Between(a, b, true) }
	} else {
		all = r.All
		bound = func(a, b time.Time) []time.Time { return r.Between(a, b, true) }
	}

	if c.Limit <= 0 {
		return all(), nil
	}
	if len(want) == 0 {
		return nil, nil
	}
	// Unbounded rule: read a window that provably covers the expected prefix,
	// then truncate. This terminates even for infinite rules.
	got := bound(start.Add(-time.Second), want[len(want)-1].Add(time.Second))
	if len(got) > c.Limit {
		got = got[:c.Limit]
	}
	return got, nil
}

// runCase replays a single upstream case under a timeout and returns an empty
// string on success or a human-readable failure reason.
func runCase(c upstreamCase) (reason string) {
	done := make(chan string, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Sprintf("panic: %v", p)
			}
		}()
		done <- compareCase(c)
	}()
	select {
	case r := <-done:
		return r
	case <-time.After(caseTimeout):
		return fmt.Sprintf("timed out after %s (possible unbounded iteration)", caseTimeout)
	}
}

func compareCase(c upstreamCase) string {
	loc := time.UTC
	if c.TZID != "" {
		l, err := time.LoadLocation(c.TZID)
		if err != nil {
			return fmt.Sprintf("load location %q: %v", c.TZID, err)
		}
		loc = l
	}
	start, err := parseInstant(c.DTStart, loc)
	if err != nil {
		return fmt.Sprintf("dtstart %q: %v", c.DTStart, err)
	}
	want := make([]time.Time, 0, len(c.Expected))
	for _, e := range c.Expected {
		ts, err := parseInstant(e, loc)
		if err != nil {
			return fmt.Sprintf("expected %q: %v", e, err)
		}
		want = append(want, ts)
	}

	got, err := buildOccurrences(c, start, want, loc)
	if err != nil {
		return err.Error()
	}
	if len(got) != len(want) {
		return fmt.Sprintf("got %d occurrences, want %d\n got: %s\nwant: %s",
			len(got), len(want), formatTimes(got), formatTimes(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			return fmt.Sprintf("occurrence %d = %s, want %s",
				i, got[i].UTC().Format(time.RFC3339), want[i].UTC().Format(time.RFC3339))
		}
	}
	return ""
}

// formatTimes renders at most the first eight instants of ts for diagnostics.
func formatTimes(ts []time.Time) string {
	const max = 8
	parts := make([]string, 0, max+1)
	for i, t := range ts {
		if i == max {
			parts = append(parts, fmt.Sprintf("...(+%d more)", len(ts)-max))
			break
		}
		parts = append(parts, t.UTC().Format(time.RFC3339))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// corpus pairs a subtest namespace with its vendored JSON file.
type corpus struct {
	name string
	path string
}

var corpora = []corpus{
	{"dateutil", "testdata/dateutil-rrule-cases.json"},
	{"rfc5545", "testdata/rfc5545-examples.json"},
}

// TestUpstreamParity replays every vendored upstream case as its own subtest,
// named after the upstream case ID, so a failure names the exact upstream
// vector. Cases listed in knownGaps are skipped rather than failed.
func TestUpstreamParity(t *testing.T) {
	for _, c := range corpora {
		c := c
		t.Run(c.name, func(t *testing.T) {
			for _, tc := range loadCorpus(t, c.path) {
				tc := tc
				id := c.name + "/" + tc.ID
				t.Run(tc.ID, func(t *testing.T) {
					t.Parallel()
					reason := runCase(tc)
					if gap, ok := knownGaps[id]; ok {
						if reason == "" {
							t.Fatalf("listed in knownGaps (%s) but now passes; remove the entry", gap)
						}
						t.Skipf("known gap: %s (%s)", gap, firstLine(reason))
						return
					}
					if reason != "" {
						t.Fatalf("%s\nRRULE:%s\nDTSTART:%s", reason, tc.RRule, tc.DTStart)
					}
				})
			}
		})
	}
}

// TestUpstreamParityScore measures and logs the honest per-corpus pass rate over
// the vendored upstream cases. Entries in knownGaps count as failures here, so
// the number reported in parity.json is never inflated by the allowlist.
func TestUpstreamParityScore(t *testing.T) {
	totalPass, totalAll := 0, 0
	for _, c := range corpora {
		cases := loadCorpus(t, c.path)
		pass := 0
		var failures []string
		for _, tc := range cases {
			if runCase(tc) == "" {
				pass++
			} else {
				failures = append(failures, tc.ID)
			}
		}
		totalPass += pass
		totalAll += len(cases)
		t.Logf("%s: %d/%d cases pass (%.1f%%)", c.name, pass, len(cases),
			100*float64(pass)/float64(len(cases)))
		sort.Strings(failures)
		if len(failures) > 0 {
			t.Logf("%s failing cases (%d): %s", c.name, len(failures), strings.Join(failures, " "))
		}
	}
	t.Logf("TOTAL upstream parity: %d/%d (%.1f%%)", totalPass, totalAll,
		100*float64(totalPass)/float64(totalAll))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
