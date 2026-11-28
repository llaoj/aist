package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSeparatorLineWidth(t *testing.T) {
	if got := len(SeparatorLine()); got != separatorCount {
		t.Fatalf("SeparatorLine len = %d, want %d", got, separatorCount)
	}

	zhTitle := "欢迎使用 AI Shell assistanT (AIST) v0.7.9"
	enTitle := "Welcome to AI Shell assistanT (AIST) v0.7.9"
	for _, line := range []string{zhTitle, enTitle} {
		want := lipgloss.Width(line)
		got := lipgloss.Width(SeparatorLineWidth(want))
		if got != want {
			t.Fatalf("SeparatorLineWidth for %q: width = %d, want %d", line, got, want)
		}
	}
}
