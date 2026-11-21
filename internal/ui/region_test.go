package ui

import "testing"

func TestRemoveBlankLines(t *testing.T) {
	src := "\nfoo\n\nbar\n  \nbaz\n\n"
	want := "foo\nbar\nbaz"
	if got := RemoveBlankLines(src); got != want {
		t.Fatalf("RemoveBlankLines() = %q, want %q", got, want)
	}
}
