package parse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Since parses a duration string or absolute date string into a time.Time
// representing the start of a time range.
//
// Accepted formats:
//
//	"7d"                  — 7 days ago from now
//	"30d"                 — 30 days ago from now
//	"2024-01-01"          — absolute date (UTC midnight)
//	"2024-01-01T15:04:05" — absolute datetime (UTC)
func Since(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	// relative duration: Nd
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("invalid duration %q: expected format like 7d", s)
		}
		return time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour), nil
	}

	// absolute datetime
	if strings.Contains(s, "T") {
		t, err := time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid datetime %q: expected 2006-01-02T15:04:05", s)
		}
		return t.UTC(), nil
	}

	// absolute date
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected 2006-01-02", s)
	}
	return t.UTC(), nil
}

// Size parses a human-readable size string into a number of bytes.
//
// Accepted formats:
//
//	"1024"      — raw bytes
//	"10kb"      — kilobytes (case-insensitive)
//	"5mb"       — megabytes
//	"1gb"       — gigabytes
func Size(s string) (int64, error) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	multipliers := []struct {
		suffix string
		factor int64
	}{
		{"gb", 1 << 30},
		{"mb", 1 << 20},
		{"kb", 1 << 10},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(lower, m.suffix) {
			numStr := strings.TrimSuffix(lower, m.suffix)
			n, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid size %q", s)
			}
			return int64(n * float64(m.factor)), nil
		}
	}

	// raw bytes
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q: expected bytes or suffix kb/mb/gb", s)
	}
	return n, nil
}
