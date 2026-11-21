package util

import "testing"

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
