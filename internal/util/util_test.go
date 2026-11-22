package util

import (
	"testing"
	"time"
)

func TestTruncatePrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "short", in: "abc", max: 5, want: "abc"},
		{name: "ascii", in: "abcdef", max: 3, want: "abc…[truncated 3 bytes]"},
		{name: "keeps runes whole", in: "中文字", max: 4, want: "中…[truncated 6 bytes]"},
		{name: "zero", in: "abc", max: 0, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncatePrefix(tc.in, tc.max); got != tc.want {
				t.Fatalf("TruncatePrefix(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

func TestFormatThousands(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 42180, want: "42,180"},
		{in: 500000, want: "500,000"},
		{in: 12345678, want: "12,345,678"},
		{in: -999, want: "-999"},
		{in: -12345, want: "-12,345"},
	}
	for _, tc := range cases {
		if got := FormatThousands(tc.in); got != tc.want {
			t.Fatalf("FormatThousands(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatLocalMinute(t *testing.T) {
	if got := FormatLocalMinute(time.Time{}); got != "" {
		t.Fatalf("FormatLocalMinute(zero) = %q, want empty", got)
	}
	ts := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	if got, want := FormatLocalMinute(ts), ts.Local().Format("2006-01-02 15:04"); got != want {
		t.Fatalf("FormatLocalMinute = %q, want %q", got, want)
	}
}
