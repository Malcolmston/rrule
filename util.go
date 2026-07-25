package rrule

import "sort"

// sortedUnique returns the values of s, sorted ascending with duplicates
// removed. It returns nil for an empty input so that "rule part absent" and
// "rule part empty" stay indistinguishable, as they are in RFC 5545.
func sortedUnique(s []int) []int {
	if len(s) == 0 {
		return nil
	}
	out := make([]int, len(s))
	copy(out, s)
	sort.Ints(out)
	n := 0
	for i, v := range out {
		if i == 0 || v != out[n-1] {
			out[n] = v
			n++
		}
	}
	return out[:n]
}

// containsInt reports whether v appears in s.
func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// gcd returns the greatest common divisor of a and b.
func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// mod is Python's % : the result carries the sign of the divisor.
func mod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

// divmod is Python's divmod: floor division together with mod.
func divmod(a, b int) (int, int) {
	m := mod(a, b)
	return (a - m) / b, m
}

// isLeap reports whether year is a Gregorian leap year.
func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// daysInYear returns 365 or 366.
func daysInYear(year int) int {
	if isLeap(year) {
		return 366
	}
	return 365
}

// monthLengths holds the number of days in each month of a common year.
var monthLengths = [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// daysInMonth returns the number of days in the given 1-based month.
func daysInMonth(year, month int) int {
	if month == 2 && isLeap(year) {
		return 29
	}
	return monthLengths[month-1]
}
