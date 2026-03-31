package parse_test

import (
	"testing"
	"time"

	"github.com/bjluckow/fsvector/pkg/parse"
)

func TestSince_Relative(t *testing.T) {
	cases := []struct {
		input string
		days  int
	}{
		{"7d", 7},
		{"30d", 30},
		{"1d", 1},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parse.Since(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expected := time.Now().UTC().Add(-time.Duration(tc.days) * 24 * time.Hour)
			diff := expected.Sub(got)
			if diff < 0 {
				diff = -diff
			}
			if diff > time.Second {
				t.Errorf("got %v, want ~%v (diff %v)", got, expected, diff)
			}
		})
	}
}

func TestSince_AbsoluteDate(t *testing.T) {
	got, err := parse.Since("2024-01-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSince_AbsoluteDatetime(t *testing.T) {
	got, err := parse.Since("2024-06-15T12:30:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSince_Invalid(t *testing.T) {
	cases := []string{"abc", "0d", "-7d", "01-01-2024", ""}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := parse.Since(tc)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc)
			}
		})
	}
}

func TestSize_Bytes(t *testing.T) {
	got, err := parse.Size("1024")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1024 {
		t.Errorf("got %d, want 1024", got)
	}
}

func TestSize_Kilobytes(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"10kb", 10240},
		{"10KB", 10240},
		{"10kB", 10240},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parse.Size(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSize_Megabytes(t *testing.T) {
	got, err := parse.Size("5mb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5*1024*1024 {
		t.Errorf("got %d, want %d", got, 5*1024*1024)
	}
}

func TestSize_Gigabytes(t *testing.T) {
	got, err := parse.Size("1gb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1<<30 {
		t.Errorf("got %d, want %d", got, int64(1<<30))
	}
}

func TestSize_Invalid(t *testing.T) {
	cases := []string{"abc", "-1", "10tb", ""}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := parse.Size(tc)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc)
			}
		})
	}
}
