package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
)

func TestColorizeDiffKeepsTextAndAddsColours(t *testing.T) {
	old := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = old }()

	diff := "--- a.txt\n+++ a.txt\n@@ -1,3 +1,3 @@\n context\n-old\n+new\n\\ No newline at end of file"
	out := ColorizeDiff(diff)

	if got := ansi.Strip(out); got != diff {
		t.Fatalf("ColorizeDiff changed the diff text:\n got %q\nwant %q", got, diff)
	}
	for name, code := range map[string]string{
		"removed": "\x1b[31m",
		"added":   "\x1b[32m",
		"hunk":    "\x1b[36m",
	} {
		if !strings.Contains(out, code) {
			t.Fatalf("%s line is not coloured with %q: %q", name, code, out)
		}
	}
	if !strings.HasPrefix(out, "\x1b[1;96m--- a.txt") {
		t.Fatalf("file header is not highlighted: %q", out)
	}
}

func TestColorizeDiffDisabledWithoutTerminal(t *testing.T) {
	old := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = old }()

	diff := "--- a.txt\n+++ a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	if got := ColorizeDiff(diff); got != diff {
		t.Fatalf("ColorizeDiff added escape codes when colour is disabled: %q", got)
	}
}

func TestColorizeDiffEmpty(t *testing.T) {
	if got := ColorizeDiff(""); got != "" {
		t.Fatalf("ColorizeDiff(%q) = %q", "", got)
	}
}
