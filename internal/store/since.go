package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseSince turns a human bound into a time: a relative window such as
// "30m", "24h", "7d", "2w", "3mo" (measured back from now) or an absolute
// date/time "2026-09-01", "2026-09-01T10:00" (local time), or RFC3339.
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if d, ok := parseWindow(s); ok {
		return now.Add(-d), nil
	}
	for _, l := range []string{"2006-01-02", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(l, s, now.Location()); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q: use 7d, 24h, 2w, 3mo, or 2026-09-01[T10:00]", s)
}

func parseWindow(s string) (time.Duration, bool) {
	units := []struct {
		suffix string
		unit   time.Duration
	}{
		{"mo", 30 * 24 * time.Hour},
		{"w", 7 * 24 * time.Hour},
		{"d", 24 * time.Hour},
		{"h", time.Hour},
		{"m", time.Minute},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil || n < 0 {
				return 0, false
			}
			return time.Duration(n * float64(u.unit)), true
		}
	}
	return 0, false
}
