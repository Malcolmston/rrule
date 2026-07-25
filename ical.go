package rrule

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	// Embedding the IANA time zone database keeps TZID resolution — and so
	// daylight-saving behaviour — identical on hosts that ship no zoneinfo.
	// It is only consulted when the system database is missing.
	_ "time/tzdata"
)

// Property is a raw iCalendar content line: a name, a value, and the
// parameters that qualified it, such as TZID or VALUE.
type Property struct {
	// Name is the upper-case property name, e.g. "DTSTART".
	Name string
	// Value is the property value, with escapes still in place.
	Value string
	// Params holds the property parameters keyed by upper-case name; a
	// parameter may carry several comma-separated values.
	Params map[string][]string
}

// Event is a VEVENT component: the properties this package models directly,
// plus every other property it carried, kept verbatim in Props.
type Event struct {
	// UID identifies the event.
	UID string
	// Summary is the SUMMARY text, unescaped.
	Summary string
	// Description is the DESCRIPTION text, unescaped.
	Description string
	// Location is the LOCATION text, unescaped.
	Location string
	// DTStart is the start of the event.
	DTStart time.Time
	// DTEnd is the end of the event, zero when the event carried none.
	DTEnd time.Time
	// AllDay reports that DTSTART was a DATE rather than a DATE-TIME.
	AllDay bool
	// RRule is the event's recurrence rule, nil when it does not recur.
	RRule *RRule
	// Set is the full recurrence set of the event — its RRULE together with
	// any RDATE, EXRULE and EXDATE — and is nil when the event does not
	// recur.
	Set *Set
	// Props holds every property of the component, including the ones parsed
	// into the fields above, keyed by upper-case name.
	Props map[string][]Property
}

// Calendar is a VCALENDAR component holding events.
type Calendar struct {
	// ProdID is the PRODID property.
	ProdID string
	// Version is the VERSION property, normally "2.0".
	Version string
	// Events are the VEVENT components of the calendar, in document order.
	Events []*Event
}

// defaultProdID identifies calendars this package writes.
const defaultProdID = "-//malcolmston//go rrule//EN"

// ParseCalendar reads an iCalendar object. Folded lines are unfolded per
// RFC 5545 §3.1 — a CRLF followed by a space or tab continues the previous
// line — property parameters are decoded, and DTSTART, DTEND, RRULE, RDATE,
// EXRULE and EXDATE are turned into times, rules and recurrence sets.
// Components other than VEVENT (VTIMEZONE, VALARM, ...) are skipped, but
// their content lines are not mistaken for the enclosing component's.
func ParseCalendar(r io.Reader) (*Calendar, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	lines := unfold(string(data))

	cal := &Calendar{}
	var stack []string
	var ev *Event
	inCalendar := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, params, value, err := splitContentLine(line)
		if err != nil {
			return nil, err
		}
		name = strings.ToUpper(name)
		if name == "" {
			return nil, fmt.Errorf("%w: content line without a name: %q", ErrInvalidCalendar, line)
		}

		switch name {
		case "BEGIN":
			comp := strings.ToUpper(value)
			stack = append(stack, comp)
			switch {
			case comp == "VCALENDAR" && len(stack) == 1:
				inCalendar = true
			case comp == "VEVENT" && ev == nil:
				ev = &Event{Props: map[string][]Property{}}
			}
			continue
		case "END":
			comp := strings.ToUpper(value)
			if len(stack) == 0 || stack[len(stack)-1] != comp {
				return nil, fmt.Errorf("%w: END:%s does not match the open component", ErrInvalidCalendar, comp)
			}
			stack = stack[:len(stack)-1]
			if comp == "VEVENT" && ev != nil {
				if err := finishEvent(ev); err != nil {
					return nil, err
				}
				cal.Events = append(cal.Events, ev)
				ev = nil
			}
			continue
		}

		if ev != nil && len(stack) > 0 && stack[len(stack)-1] == "VEVENT" {
			ev.Props[name] = append(ev.Props[name], Property{Name: name, Value: value, Params: params})
			continue
		}
		if inCalendar && len(stack) == 1 {
			switch name {
			case "PRODID":
				cal.ProdID = unescapeText(value)
			case "VERSION":
				cal.Version = value
			}
		}
	}

	if len(stack) != 0 {
		return nil, fmt.Errorf("%w: unterminated component %s", ErrInvalidCalendar, stack[len(stack)-1])
	}
	if !inCalendar {
		return nil, fmt.Errorf("%w: no VCALENDAR component", ErrInvalidCalendar)
	}
	return cal, nil
}

// finishEvent decodes the modelled properties of a parsed VEVENT.
func finishEvent(ev *Event) error {
	if p, ok := first(ev.Props, "UID"); ok {
		ev.UID = unescapeText(p.Value)
	}
	if p, ok := first(ev.Props, "SUMMARY"); ok {
		ev.Summary = unescapeText(p.Value)
	}
	if p, ok := first(ev.Props, "DESCRIPTION"); ok {
		ev.Description = unescapeText(p.Value)
	}
	if p, ok := first(ev.Props, "LOCATION"); ok {
		ev.Location = unescapeText(p.Value)
	}

	loc := time.UTC
	if p, ok := first(ev.Props, "DTSTART"); ok {
		t, allDay, err := parseDateTimeProp(p.Value, p.Params, time.UTC)
		if err != nil {
			return err
		}
		ev.DTStart = t
		ev.AllDay = allDay
		loc = t.Location()
	}
	if p, ok := first(ev.Props, "DTEND"); ok {
		t, _, err := parseDateTimeProp(p.Value, p.Params, loc)
		if err != nil {
			return err
		}
		ev.DTEnd = t
	}

	rules := ev.Props["RRULE"]
	exrules := ev.Props["EXRULE"]
	rdates := ev.Props["RDATE"]
	exdates := ev.Props["EXDATE"]
	if len(rules) == 0 && len(exrules) == 0 && len(rdates) == 0 && len(exdates) == 0 {
		return nil
	}

	set := NewSet()
	for i, p := range rules {
		opts := Options{DTStart: ev.DTStart}
		if _, err := applyRuleParts(&opts, p.Value); err != nil {
			return err
		}
		fixNaiveUntil(&opts)
		r, err := New(opts)
		if err != nil {
			return err
		}
		if i == 0 {
			ev.RRule = r
		}
		set.RRule(r)
	}
	for _, p := range exrules {
		opts := Options{DTStart: ev.DTStart}
		if _, err := applyRuleParts(&opts, p.Value); err != nil {
			return err
		}
		fixNaiveUntil(&opts)
		r, err := New(opts)
		if err != nil {
			return err
		}
		set.ExRule(r)
	}
	for _, p := range rdates {
		ts, err := parseDateList(p, loc)
		if err != nil {
			return err
		}
		for _, t := range ts {
			set.RDate(t)
		}
	}
	for _, p := range exdates {
		ts, err := parseDateList(p, loc)
		if err != nil {
			return err
		}
		for _, t := range ts {
			set.ExDate(t)
		}
	}
	ev.Set = set
	return nil
}

// fixNaiveUntil re-anchors an UNTIL that was read as UTC by default into
// DTSTART's location when DTSTART is not itself in UTC.
func fixNaiveUntil(opts *Options) {
	if opts.Until.IsZero() || opts.DTStart.IsZero() {
		return
	}
	if opts.DTStart.Location() == time.UTC || opts.Until.Location() != time.UTC {
		return
	}
	u := opts.Until
	opts.Until = time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, opts.DTStart.Location())
}

// parseDateList parses a comma-separated RDATE or EXDATE value.
func parseDateList(p Property, loc *time.Location) ([]time.Time, error) {
	var out []time.Time
	for _, v := range strings.Split(p.Value, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		// A PERIOD value carries "start/end"; only the start is an occurrence.
		if i := strings.IndexByte(v, '/'); i >= 0 {
			v = v[:i]
		}
		t, _, err := parseDateTimeProp(v, p.Params, loc)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// first returns the first property recorded under name.
func first(props map[string][]Property, name string) (Property, bool) {
	if p, ok := props[name]; ok && len(p) > 0 {
		return p[0], true
	}
	return Property{}, false
}

// Encode writes the calendar as an iCalendar object: CRLF line endings and
// content lines folded at 75 octets per RFC 5545 §3.1.
func (c *Calendar) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	version := c.Version
	if version == "" {
		version = "2.0"
	}
	prodID := c.ProdID
	if prodID == "" {
		prodID = defaultProdID
	}

	writeLine(bw, "BEGIN:VCALENDAR")
	writeLine(bw, "VERSION:"+version)
	writeLine(bw, "PRODID:"+escapeText(prodID))
	for _, ev := range c.Events {
		if err := ev.encode(bw); err != nil {
			return err
		}
	}
	writeLine(bw, "END:VCALENDAR")
	return bw.Flush()
}

// modelledProps are emitted from the Event fields, so a copy carried in Props
// must not be written a second time.
var modelledProps = map[string]bool{
	"UID": true, "SUMMARY": true, "DESCRIPTION": true, "LOCATION": true,
	"DTSTART": true, "DTEND": true, "RRULE": true, "EXRULE": true,
	"RDATE": true, "EXDATE": true,
}

// encode writes one VEVENT component.
func (ev *Event) encode(bw *bufio.Writer) error {
	writeLine(bw, "BEGIN:VEVENT")
	if ev.UID != "" {
		writeLine(bw, "UID:"+escapeText(ev.UID))
	}
	if !ev.DTStart.IsZero() {
		writeLine(bw, dateProp("DTSTART", ev.DTStart, ev.AllDay))
	}
	if !ev.DTEnd.IsZero() {
		writeLine(bw, dateProp("DTEND", ev.DTEnd, ev.AllDay))
	}
	if ev.Summary != "" {
		writeLine(bw, "SUMMARY:"+escapeText(ev.Summary))
	}
	if ev.Description != "" {
		writeLine(bw, "DESCRIPTION:"+escapeText(ev.Description))
	}
	if ev.Location != "" {
		writeLine(bw, "LOCATION:"+escapeText(ev.Location))
	}

	set := ev.Set
	if set == nil && ev.RRule != nil {
		set = NewSet()
		set.RRule(ev.RRule)
	}
	if set != nil {
		for _, r := range set.RRules() {
			writeLine(bw, "RRULE:"+r.RuleString())
		}
		for _, r := range set.ExRules() {
			writeLine(bw, "EXRULE:"+r.RuleString())
		}
		if d := set.RDates(); len(d) > 0 {
			writeLine(bw, "RDATE:"+joinTimes(d))
		}
		if d := set.ExDates(); len(d) > 0 {
			writeLine(bw, "EXDATE:"+joinTimes(d))
		}
	}

	names := make([]string, 0, len(ev.Props))
	for name := range ev.Props {
		if !modelledProps[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, p := range ev.Props[name] {
			writeLine(bw, p.line())
		}
	}
	writeLine(bw, "END:VEVENT")
	return nil
}

// line renders the property back into a content line, parameters included.
func (p Property) line() string {
	var b strings.Builder
	b.WriteString(p.Name)
	names := make([]string, 0, len(p.Params))
	for k := range p.Params {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		b.WriteByte(';')
		b.WriteString(k)
		b.WriteByte('=')
		for i, v := range p.Params[k] {
			if i > 0 {
				b.WriteByte(',')
			}
			if strings.ContainsAny(v, ":;,") {
				b.WriteByte('"')
				b.WriteString(v)
				b.WriteByte('"')
			} else {
				b.WriteString(v)
			}
		}
	}
	b.WriteByte(':')
	b.WriteString(p.Value)
	return b.String()
}

// dateProp renders a DTSTART or DTEND content line, with a VALUE=DATE
// parameter for an all-day event and a TZID parameter for a named zone.
func dateProp(name string, t time.Time, allDay bool) string {
	if allDay {
		return name + ";VALUE=DATE:" + t.Format("20060102")
	}
	switch loc := t.Location(); loc {
	case time.UTC:
		return name + ":" + t.Format("20060102T150405") + "Z"
	case time.Local:
		return name + ":" + t.Format("20060102T150405")
	default:
		return name + ";TZID=" + loc.String() + ":" + t.Format("20060102T150405")
	}
}

// unfold splits iCalendar text into logical lines, undoing RFC 5545 folding:
// a CRLF (or bare LF) followed by a space or tab continues the previous line.
func unfold(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

// maxLineOctets is the RFC 5545 limit on the octet length of a content line,
// counting the leading space of a continuation but not the CRLF.
const maxLineOctets = 75

// writeLine writes one content line, folded at maxLineOctets octets with CRLF
// line breaks. Folding counts octets, not runes, and never splits a UTF-8
// sequence: the break moves back to a rune boundary instead.
func writeLine(w *bufio.Writer, line string) {
	b := []byte(line)
	limit := maxLineOctets
	for {
		if len(b) <= limit {
			w.Write(b)
			w.WriteString("\r\n")
			return
		}
		n := limit
		for n > 0 && b[n]&0xC0 == 0x80 {
			n--
		}
		if n == 0 {
			// A single rune longer than the limit cannot be split at all.
			n = limit
		}
		w.Write(b[:n])
		w.WriteString("\r\n")
		w.WriteByte(' ')
		b = b[n:]
		limit = maxLineOctets - 1
	}
}

// splitContentLine splits "NAME;PARAM=value:VALUE" into its three pieces.
// Parameter values may be double quoted, in which case they may contain the
// ':' and ';' delimiters. A line holding no name — a bare RRULE value such as
// "FREQ=DAILY;COUNT=3" — yields an empty name and the whole line as the value.
func splitContentLine(line string) (name string, params map[string][]string, value string, err error) {
	inQuote := false
	colon := -1
	semi := -1
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				colon = i
			}
		case ';':
			if !inQuote && semi < 0 {
				semi = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon < 0 {
		return "", nil, line, nil
	}
	head, value := line[:colon], line[colon+1:]
	if semi < 0 || semi > colon {
		return strings.TrimSpace(head), nil, value, nil
	}
	name = strings.TrimSpace(head[:semi])
	params = map[string][]string{}
	for _, p := range splitUnquoted(head[semi+1:], ';') {
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return "", nil, "", fmt.Errorf("%w: parameter %q is not NAME=VALUE", ErrInvalidCalendar, p)
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		for _, one := range splitUnquoted(v, ',') {
			params[k] = append(params[k], strings.Trim(one, `"`))
		}
	}
	return name, params, value, nil
}

// splitUnquoted splits s on sep, ignoring separators inside double quotes.
func splitUnquoted(s string, sep byte) []string {
	var out []string
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case sep:
			if !inQuote {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// escapeText applies the RFC 5545 TEXT escaping rules: backslash, comma,
// semicolon and newline become "\\", "\,", "\;" and "\n".
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case ',':
			b.WriteString(`\,`)
		case ';':
			b.WriteString(`\;`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// A bare CR carries no meaning in TEXT; drop it, and let a CRLF
			// be written as the single "\n" escape.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unescapeText reverses escapeText, leaving unknown escapes as their escaped
// character so that no information is lost.
func unescapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n', 'N':
			b.WriteByte('\n')
		case '\\', ',', ';':
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
