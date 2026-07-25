package rrule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse reads one RFC 5545 recurrence rule, with or without the "RRULE:"
// property name:
//
//	rrule.Parse("FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6")
//	rrule.Parse("RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6")
//
// A leading DTSTART line is accepted as well, so the output of String round
// trips. When no DTSTART is present the rule is anchored at the current local
// time truncated to the second, as dateutil does.
func Parse(line string) (*RRule, error) {
	return parseBlock(line, false)
}

// StrToRRule parses a full DTSTART + RRULE block, the form String produces and
// the form an iCalendar object carries:
//
//	rrule.StrToRRule("DTSTART:19970902T090000\nRRULE:FREQ=DAILY;COUNT=10")
//
// Folded lines are unfolded first. It returns an error wrapping ErrNoRRule if
// the block holds no rule. A block that also carries RDATE, EXRULE or EXDATE
// describes a recurrence set rather than a single rule; pass it to StrToSet.
func StrToRRule(s string) (*RRule, error) {
	return parseBlock(s, true)
}

// StrToSet parses a block of RFC 5545 content lines into a recurrence set. It
// accepts DTSTART together with any number of RRULE, EXRULE, RDATE and EXDATE
// lines, which is what an iCalendar VEVENT's recurrence properties amount to:
//
//	s, err := rrule.StrToSet(
//	        "DTSTART:19970902T090000\n" +
//	        "RRULE:FREQ=YEARLY;COUNT=6;BYDAY=TU,TH\n" +
//	        "EXDATE:19970904T090000")
func StrToSet(s string) (*Set, error) {
	b, err := parseICalBlock(s)
	if err != nil {
		return nil, err
	}
	set := NewSet()
	for _, raw := range b.rrules {
		r, err := b.rule(raw)
		if err != nil {
			return nil, err
		}
		set.RRule(r)
	}
	for _, raw := range b.exrules {
		r, err := b.rule(raw)
		if err != nil {
			return nil, err
		}
		set.ExRule(r)
	}
	for _, t := range b.rdates {
		set.RDate(t)
	}
	for _, t := range b.exdates {
		set.ExDate(t)
	}
	return set, nil
}

// icalBlock is the decoded content of a DTSTART + recurrence-property block.
type icalBlock struct {
	dtstart   time.Time
	haveStart bool
	rrules    []string
	exrules   []string
	rdates    []time.Time
	exdates   []time.Time
}

// rule compiles one raw RRULE or EXRULE value against the block's DTSTART.
func (b *icalBlock) rule(value string) (*RRule, error) {
	opts := Options{DTStart: b.dtstart}
	naive, err := applyRuleParts(&opts, value)
	if err != nil {
		return nil, err
	}
	if b.haveStart && naive {
		u := opts.Until
		opts.Until = time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, opts.DTStart.Location())
	}
	return New(opts)
}

// recurrenceProps are the property names a recurrence block may hold. None of
// their values can contain whitespace, which is what lets splitRecurrenceLines
// accept several of them on one line.
var recurrenceProps = []string{"DTSTART", "RRULE", "EXRULE", "RDATE", "EXDATE"}

// splitRecurrenceLines unfolds a block and splits it into content lines. It is
// deliberately lenient in the way dateutil's rrulestr is: properties separated
// by spaces rather than by line breaks are accepted, because none of the
// values involved may contain whitespace.
func splitRecurrenceLines(s string) []string {
	var out []string
	for _, line := range unfold(s) {
		var cur string
		for _, tok := range strings.Fields(line) {
			if startsProperty(tok) {
				if cur != "" {
					out = append(out, cur)
				}
				cur = tok
				continue
			}
			if cur == "" {
				cur = tok
			} else {
				cur += " " + tok
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

// startsProperty reports whether tok begins with a recurrence property name
// followed by a ':' or ';'.
func startsProperty(tok string) bool {
	up := strings.ToUpper(tok)
	for _, name := range recurrenceProps {
		if len(up) > len(name) && strings.HasPrefix(up, name) &&
			(up[len(name)] == ':' || up[len(name)] == ';') {
			return true
		}
	}
	return false
}

// parseICalBlock unfolds and splits a block of recurrence content lines.
func parseICalBlock(s string) (*icalBlock, error) {
	b := &icalBlock{}
	// DTSTART must be read before the rules that depend on its location, so
	// the lines are visited in two passes.
	type parsed struct {
		name   string
		params map[string][]string
		value  string
	}
	var lines []parsed
	for _, line := range splitRecurrenceLines(s) {
		name, params, value, err := splitContentLine(line)
		if err != nil {
			return nil, err
		}
		name = strings.ToUpper(name)
		if name == "" {
			// A bare rule value, e.g. "FREQ=DAILY;COUNT=3".
			name, value = "RRULE", line
		}
		lines = append(lines, parsed{name, params, value})
		if name == "DTSTART" {
			t, _, err := parseDateTimeProp(value, params, time.Local)
			if err != nil {
				return nil, err
			}
			b.dtstart = t
			b.haveStart = true
		}
	}

	loc := time.Local
	if b.haveStart {
		loc = b.dtstart.Location()
	}
	for _, l := range lines {
		switch l.name {
		case "DTSTART":
		case "RRULE":
			b.rrules = append(b.rrules, l.value)
		case "EXRULE":
			b.exrules = append(b.exrules, l.value)
		case "RDATE", "EXDATE":
			ts, err := parseDateList(Property{Name: l.name, Value: l.value, Params: l.params}, loc)
			if err != nil {
				return nil, err
			}
			if l.name == "RDATE" {
				b.rdates = append(b.rdates, ts...)
			} else {
				b.exdates = append(b.exdates, ts...)
			}
		default:
			return nil, fmt.Errorf("%w: unexpected property %q", ErrParse, l.name)
		}
	}
	return b, nil
}

// parseBlock parses one or more content lines into a single rule. When
// requireRRule is set the input must carry an actual rule, not just a DTSTART.
func parseBlock(s string, requireRRule bool) (*RRule, error) {
	b, err := parseICalBlock(s)
	if err != nil {
		return nil, err
	}
	if len(b.rrules) == 0 {
		if requireRRule {
			return nil, fmt.Errorf("%w: in %q", ErrNoRRule, s)
		}
		return nil, fmt.Errorf("%w: no rule parts in %q", ErrParse, s)
	}
	if len(b.exrules) > 0 || len(b.rdates) > 0 || len(b.exdates) > 0 {
		return nil, fmt.Errorf("%w: block describes a recurrence set, use StrToSet", ErrParse)
	}
	if len(b.rrules) > 1 {
		return nil, fmt.Errorf("%w: block holds %d rules, use StrToSet", ErrParse, len(b.rrules))
	}
	return b.rule(b.rrules[0])
}

// applyRuleParts parses the semicolon-separated rule parts of an RRULE value
// into opts. It reports whether an UNTIL was present without a UTC designator,
// which the caller re-anchors in DTSTART's location.
func applyRuleParts(opts *Options, value string) (bool, error) {
	seen := map[string]bool{}
	naive := false
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return naive, fmt.Errorf("%w: rule part %q is not NAME=VALUE", ErrParse, part)
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if seen[k] {
			return naive, fmt.Errorf("%w: duplicate rule part %s", ErrParse, k)
		}
		seen[k] = true

		var err error
		switch k {
		case "FREQ":
			opts.Freq, err = parseFreq(strings.ToUpper(v))
		case "INTERVAL":
			opts.Interval, err = parseInt(k, v)
		case "COUNT":
			opts.Count, err = parseInt(k, v)
		case "UNTIL":
			var t time.Time
			t, _, err = parseDateTimeProp(v, nil, time.UTC)
			opts.Until = t
			if err == nil && !strings.HasSuffix(strings.ToUpper(v), "Z") {
				naive = true
			}
		case "WKST":
			var w Weekday
			w, err = parseWeekday(v)
			if err == nil {
				if w.Day == time.Sunday {
					opts.Wkst = SundayStart
				} else {
					opts.Wkst = w.Day
				}
			}
		case "BYSETPOS":
			opts.BySetPos, err = parseIntList(k, v)
		case "BYMONTH":
			opts.ByMonth, err = parseIntList(k, v)
		case "BYMONTHDAY":
			opts.ByMonthDay, err = parseIntList(k, v)
		case "BYYEARDAY":
			opts.ByYearDay, err = parseIntList(k, v)
		case "BYWEEKNO":
			opts.ByWeekNo, err = parseIntList(k, v)
		case "BYDAY", "BYWEEKDAY":
			opts.ByWeekday, err = parseWeekdayList(v)
		case "BYHOUR":
			opts.ByHour, err = parseIntList(k, v)
		case "BYMINUTE":
			opts.ByMinute, err = parseIntList(k, v)
		case "BYSECOND":
			opts.BySecond, err = parseIntList(k, v)
		default:
			err = fmt.Errorf("%w: unknown rule part %q", ErrParse, k)
		}
		if err != nil {
			return naive, err
		}
	}
	return naive, nil
}

// parseInt parses a single integer rule part value.
func parseInt(name, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not an integer", ErrParse, name, v)
	}
	return n, nil
}

// parseIntList parses a comma-separated list of integers.
func parseIntList(name, v string) ([]int, error) {
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := parseInt(name, strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// parseWeekdayList parses a comma-separated BYDAY value.
func parseWeekdayList(v string) ([]Weekday, error) {
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]Weekday, 0, len(parts))
	for _, p := range parts {
		w, err := parseWeekday(p)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// parseDateTimeProp parses an RFC 5545 DATE or DATE-TIME value, honouring a
// TZID or VALUE=DATE parameter. It reports whether the value was a plain date.
func parseDateTimeProp(v string, params map[string][]string, dflt *time.Location) (time.Time, bool, error) {
	loc := dflt
	if loc == nil {
		loc = time.UTC
	}
	dateOnly := false
	if params != nil {
		if vals, ok := params["VALUE"]; ok {
			for _, val := range vals {
				if strings.EqualFold(val, "DATE") {
					dateOnly = true
				}
			}
		}
		if vals, ok := params["TZID"]; ok && len(vals) > 0 {
			if l, err := time.LoadLocation(vals[0]); err == nil {
				loc = l
			}
		}
	}
	t, isDate, err := parseICalTime(v, loc)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, dateOnly || isDate, nil
}

// parseICalTime parses "19970902T090000", "19970902T090000Z" or "19970902".
func parseICalTime(v string, loc *time.Location) (time.Time, bool, error) {
	v = strings.TrimSpace(v)
	switch {
	case len(v) == 8:
		t, err := time.ParseInLocation("20060102", v, loc)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("%w: bad DATE %q", ErrParse, v)
		}
		return t, true, nil
	case len(v) == 15:
		t, err := time.ParseInLocation("20060102T150405", v, loc)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("%w: bad DATE-TIME %q", ErrParse, v)
		}
		return t, false, nil
	case len(v) == 16 && (v[15] == 'Z' || v[15] == 'z'):
		t, err := time.ParseInLocation("20060102T150405", v[:15], time.UTC)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("%w: bad DATE-TIME %q", ErrParse, v)
		}
		return t, false, nil
	default:
		return time.Time{}, false, fmt.Errorf("%w: bad date-time %q", ErrParse, v)
	}
}

// formatICalTime renders an instant in the RFC 5545 form appropriate to its
// location: with a "Z" suffix in UTC, as a local time otherwise.
func formatICalTime(t time.Time) string {
	if t.Location() == time.UTC {
		return t.Format("20060102T150405") + "Z"
	}
	return t.Format("20060102T150405")
}

// String returns the rule as RFC 5545 content lines: a DTSTART line followed
// by an RRULE line, for example
//
//	DTSTART;TZID=America/New_York:19970902T090000
//	RRULE:FREQ=DAILY;COUNT=10
//
// The output is stable and is accepted by both Parse and StrToRRule, so
// rules round trip through text.
func (r *RRule) String() string {
	var b strings.Builder
	b.WriteString(r.dtstartLine())
	b.WriteString("\n")
	b.WriteString("RRULE:")
	b.WriteString(r.RuleString())
	return b.String()
}

// dtstartLine renders the DTSTART content line of the rule, carrying a TZID
// parameter when the rule is anchored in a named zone.
func (r *RRule) dtstartLine() string {
	loc := r.dtstart.Location()
	switch loc {
	case time.UTC:
		return "DTSTART:" + r.dtstart.Format("20060102T150405") + "Z"
	case time.Local:
		return "DTSTART:" + r.dtstart.Format("20060102T150405")
	default:
		return "DTSTART;TZID=" + loc.String() + ":" + r.dtstart.Format("20060102T150405")
	}
}

// RuleString returns just the value of the RRULE property, without the
// property name and without a DTSTART line — the form that goes after
// "RRULE:" in an iCalendar object.
func (r *RRule) RuleString() string {
	o := r.orig
	parts := []string{"FREQ=" + r.freq.String()}
	if r.interval != 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(r.interval))
	}
	if r.wkst != pyWeekday(time.Monday) {
		parts = append(parts, "WKST="+weekdayNames[r.wkst])
	}
	if r.count > 0 {
		parts = append(parts, "COUNT="+strconv.Itoa(r.count))
	}
	if r.hasUntil {
		parts = append(parts, "UNTIL="+formatICalTime(r.until))
	}
	appendInts := func(name string, vals []int) {
		if len(vals) > 0 {
			parts = append(parts, name+"="+joinInts(vals))
		}
	}
	appendInts("BYSETPOS", o.BySetPos)
	appendInts("BYMONTH", o.ByMonth)
	appendInts("BYMONTHDAY", o.ByMonthDay)
	appendInts("BYYEARDAY", o.ByYearDay)
	appendInts("BYWEEKNO", o.ByWeekNo)
	if len(o.ByWeekday) > 0 {
		names := make([]string, len(o.ByWeekday))
		for i, w := range o.ByWeekday {
			names[i] = w.String()
		}
		parts = append(parts, "BYDAY="+strings.Join(names, ","))
	}
	appendInts("BYHOUR", o.ByHour)
	appendInts("BYMINUTE", o.ByMinute)
	appendInts("BYSECOND", o.BySecond)
	return strings.Join(parts, ";")
}

// joinInts renders a comma-separated integer list.
func joinInts(vals []int) string {
	s := make([]string, len(vals))
	for i, v := range vals {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, ",")
}
