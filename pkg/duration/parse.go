// Package `duration` provides parsing of durations that includes days, weeks and months.
// Likely much slower than the `time` implementation, but doesn't matter here.
package duration

import (
	"fmt"
	"strconv"
	"time"
)

const (
	Day   = 24 * time.Hour
	Week  = 7 * Day
	Month = 30 * Day
    Year  = 365 * Day
)

var unitMap = map[string]uint64{
	"ns":  uint64(time.Nanosecond),
	"us":  uint64(time.Microsecond),
	"ms":  uint64(time.Millisecond),
	"s":   uint64(time.Second),
	"m":   uint64(time.Minute),
	"min": uint64(time.Minute),
	"h":   uint64(time.Hour),
	"d":   uint64(Day),
	"w":   uint64(Week),
	"M":   uint64(Month),
    "y":   uint64(Year),
}

const (
    numbers = "0123456789"
    letters = "ABCDEFGHIJKLMNOPQRSTUVabcdefghijklmnoprstuvwxyz"
    alphanum = letters + numbers
)

// Parses a string into a duration. Unlike [time.ParseDuration], we don't deal with
// floats, so strings like "5h30m" are allowed but not "5.5h".
// We also add "d" for days, "w" for weeks, "M" for months, "y" for years. Additionally,
// "min" can be used for minutes.
func ParseDuration(s string) (time.Duration, error) {
    if s == "" || s == "0" {
        return 0, nil
    }
    
    var neg bool
    if s[0] == '-' || s[0] == '+' {
        neg = s[0] == '-'
        s = s[1:]
    }

    accum := int64(0) 
    for s != "" {
        if !isIn(s[0], alphanum) {
            return 0, fmt.Errorf("Invalid character: %s", s[:1])
        }

        // look for numbers
        var num string
        for s != "" && isIn(s[0], numbers) {
            num += s[:1]
            s = s[1:]
        }
        val, _ := strconv.Atoi(num)
        if s == "" {
            return 0, fmt.Errorf("Missing unit for number %v", val)
        }

        // look for unit
        var unit string
        for s != "" && isIn(s[0], letters) {
            unit += s[:1]
            s = s[1:]
        }
        if unit == "" {
            return 0, fmt.Errorf("Missing unit for number %v", val)
        }
        u, ok := unitMap[unit]
        if !ok {
            return 0, fmt.Errorf("bad unit: %v", unit)
        }
        
        accum += int64(val) * int64(u)
    }
    if neg {
        accum = -accum
    }

    return time.Duration(accum), nil
}

// Strings returns a string representation of the duration.
func String(d time.Duration) string {
    if d == 0 {
        return "0s"
    }

    // TODO: negatives
    u := uint64(d)
    var out string
    if d < 0 {
        u = -u
        out += "-"
    }

    out, u = fmtUnit(out, u, "y")
    out, u = fmtUnit(out, u, "M")
    out, u = fmtUnit(out, u, "w")
    out, u = fmtUnit(out, u, "d")
    out, u = fmtUnit(out, u, "h")
    out, u = fmtUnit(out, u, "m")
    out, u = fmtUnit(out, u, "s")
    out, u = fmtUnit(out, u, "ms")
    out, u = fmtUnit(out, u, "us")
    out, u = fmtUnit(out, u, "ns")

    return out
}

func isIn(b byte, s string) bool {
    for i := range s {
        if s[i] == b {
            return true
        }
    }
    return false
}

func fmtUnit(s string, u uint64, unit string) (string, uint64) {
    if u == 0 {
        return s, 0
    }
    d := unitMap[unit]
    q := u / uint64(d)
    if q != 0 {
        s += fmtInt(q)
        s += unit

        u -= q * uint64(d)
    }
    return s, u
}

func fmtInt(i uint64) string {
    return fmt.Sprintf("%d", i)
}
