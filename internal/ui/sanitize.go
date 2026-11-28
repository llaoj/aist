package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// SanitizeTerminalOutput keeps printable text and standard SGR color/style
// sequences (ESC [ ... m), stripping all other terminal control sequences
// (such as window resize, DECCOLM, screen clear, alt-screen, RIS, and OSC).
func SanitizeTerminalOutput(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	var state byte
	hasSGR := false
	rest := s
	for len(rest) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(rest, state, nil)
		state = newState
		rest = rest[n:]

		if width > 0 {
			b.WriteString(seq)
			continue
		}
		if seq == "\n" || seq == "\t" {
			b.WriteString(seq)
			continue
		}
		if isSGRSequence(seq) {
			b.WriteString(seq)
			hasSGR = true
		}
	}

	out := b.String()
	if hasSGR && !strings.HasSuffix(out, "\x1b[0m") && !strings.HasSuffix(out, "\x1b[m") {
		out += "\x1b[0m"
	}
	return out
}

func isSGRSequence(seq string) bool {
	if len(seq) < 3 || seq[0] != keyEsc || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return false
	}
	for i := 2; i < len(seq)-1; i++ {
		c := seq[i]
		if (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}
