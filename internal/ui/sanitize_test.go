package ui

import "testing"

func TestSanitizeTerminalOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text and whitespace preserved",
			in:   "line1\tcol2\r\n中文输出\n",
			want: "line1\tcol2\n中文输出\n",
		},
		{
			name: "preserves SGR color codes and resets unclosed style",
			in:   "\x1b[31merror:\x1b[0m \x1b[1;32mok",
			want: "\x1b[31merror:\x1b[0m \x1b[1;32mok\x1b[0m",
		},
		{
			name: "preserves colon-separated truecolor SGR",
			in:   "\x1b[38:2::255:128:0morange\x1b[0m",
			want: "\x1b[38:2::255:128:0morange\x1b[0m",
		},
		{
			name: "strips PuTTY resize, DECCOLM, RIS, alt-screen, clear, OSC, and private m sequences",
			in:   "\x1b[?3h\x1b[?3l\x1b[8;40;120t\x1bc\x1b[?1049h\x1b[2J\x1b[H\x1b]0;evil\x07\x1b]52;c;YWJj\x1b\\\x1b[>4;2mhello \x1b[32mworld\x1b[0m",
			want: "hello \x1b[32mworld\x1b[0m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeTerminalOutput(tc.in); got != tc.want {
				t.Fatalf("SanitizeTerminalOutput(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
