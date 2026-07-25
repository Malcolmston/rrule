package rrule

import "fmt"

// Freq is the base frequency of a recurrence rule: the RFC 5545 FREQ rule
// part. It selects the size of the interval that BYxxx rule parts expand or
// limit.
type Freq int

// The recurrence frequencies defined by RFC 5545 §3.3.10, ordered from the
// coarsest to the finest. The ordering is significant: a BYxxx rule part
// expands the candidate set when it is finer than FREQ and limits it when it
// is coarser or equal.
const (
	// Yearly repeats once per year (FREQ=YEARLY).
	Yearly Freq = iota
	// Monthly repeats once per month (FREQ=MONTHLY).
	Monthly
	// Weekly repeats once per week (FREQ=WEEKLY).
	Weekly
	// Daily repeats once per day (FREQ=DAILY).
	Daily
	// Hourly repeats once per hour (FREQ=HOURLY).
	Hourly
	// Minutely repeats once per minute (FREQ=MINUTELY).
	Minutely
	// Secondly repeats once per second (FREQ=SECONDLY).
	Secondly
)

var freqNames = [...]string{"YEARLY", "MONTHLY", "WEEKLY", "DAILY", "HOURLY", "MINUTELY", "SECONDLY"}

// String returns the RFC 5545 name of the frequency, such as "WEEKLY".
func (f Freq) String() string {
	if f < Yearly || int(f) >= len(freqNames) {
		return fmt.Sprintf("Freq(%d)", int(f))
	}
	return freqNames[f]
}

// parseFreq maps an RFC 5545 FREQ value to a Freq.
func parseFreq(s string) (Freq, error) {
	for i, n := range freqNames {
		if n == s {
			return Freq(i), nil
		}
	}
	return 0, fmt.Errorf("%w: unknown FREQ %q", ErrInvalidFreq, s)
}
